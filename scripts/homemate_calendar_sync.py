#!/usr/bin/env python3
"""
HomeMate Apple 日历同步脚本 (v3.9.12)

通过 osascript 读取 macOS Calendar.app 中 ±N 天事件，写入 SQLite calendar_events 表
（标记 source='apple_calendar'）。事件按 (source_account, external_event_id) 唯一索引去重，
循环事件用 python-dateutil 的 rrule 展开。

用法:
  python3 homemate_calendar_sync.py                       # 同步 ±30 天
  python3 homemate_calendar_sync.py --lookback 7 --lookahead 14
  HOMEMATE_DB=/path/to/homemate.db python3 homemate_calendar_sync.py

数据库路径: 环境变量 HOMEMATE_DB 优先，默认
  /Users/jarick/workspace/codebook/homework/homeassistant/homemate.db

输出格式（最后一行）: 成功: N | 跳过: M | 失败: K

TCC 权限错误（osascript -10004 / 权限违例 / not authorized）:
  退出码 2，stderr 打印 [TCC_ERROR]，需用户在「系统设置 > 隐私与安全性 > 自动化」
  中为相应进程重新勾选 Calendar。
"""

import argparse
import os
import re
import sqlite3
import subprocess
import sys
from datetime import datetime, timedelta

# ============ 配置 ============

DB_PATH = os.environ.get(
    "HOMEMATE_DB",
    "/Users/jarick/workspace/codebook/homework/homeassistant/homemate.db",
)
SOURCE = "apple_calendar"
DEFAULT_MEMBER_ID = 0  # Apple 日历事件为家庭级，不绑定具体成员

# dateutil 用于循环事件展开（可选依赖，未安装时跳过展开）
try:
    from dateutil.rrule import rrule, DAILY, WEEKLY, MONTHLY, YEARLY

    HAS_DATEUTIL = True
except ImportError:
    HAS_DATEUTIL = False

# AppleScript 频率常量 → dateutil 频率映射
FREQ_MAP = {
    "daily": DAILY,
    "weekly": WEEKLY,
    "monthly": MONTHLY,
    "yearly": YEARLY,
}


# ============ osascript: 读取 Calendar.app 事件 ============

def build_applescript(lookback_days, lookahead_days):
    """构造 AppleScript，查询 Calendar.app 中 ±N 天的事件。

    输出格式（每行一个事件，字段以 ||| 分隔）:
      calendar_name|||uid|||summary|||start_iso|||end_iso|||location|||description|||all_day|||rec_freq:::rec_interval:::rec_until:::rec_count

    日期格式: YYYY-MM-DD HH:MM:SS（在 AppleScript 内格式化，避免 locale 依赖）
    """
    return f'''
tell application "Calendar"
    set delim to "|||"
    set out to ""
    set now to current date
    set startDate to now - ({lookback_days} * days)
    set endDate to now + ({lookahead_days} * days)

    repeat with cal in calendars
        set calName to name of cal
        set evs to (every event of cal whose start date is greater than startDate and start date is less than endDate)
        repeat with ev in evs
            -- 标题
            set evSummary to ""
            try
                set evSummary to summary of ev
            end try

            -- 起止时间（格式化为 ISO）
            set evStart to start date of ev
            set evEnd to end date of ev
            set evStartStr to fmtDate(evStart)
            set evEndStr to fmtDate(evEnd)

            -- 地点
            set evLoc to ""
            try
                set evLoc to location of ev
            end try

            -- 备注
            set evDesc to ""
            try
                set evDesc to description of ev
            end try

            -- UID（稳定标识）
            set evUID to uid of ev

            -- 全天事件
            set evAllDay to "false"
            try
                if allday event of ev then
                    set evAllDay to "true"
                end if
            end try

            -- 循环规则
            set recInfo to ""
            try
                set recRule to recurrence of ev
                if recRule is not missing value then
                    set freqStr to ""
                    try
                        set freqStr to frequency of recRule as string
                    end try
                    set intVal to "1"
                    try
                        set intVal to (interval of recRule) as string
                    end try
                    set untilStr to ""
                    try
                        set untilDate to end date of recRule
                        if untilDate is not missing value then
                            set untilStr to fmtDate(untilDate)
                        end if
                    end try
                    set cntStr to "0"
                    try
                        set occCnt to occurrence count of recRule
                        if occCnt is not missing value then
                            set cntStr to (occCnt) as string
                        end if
                    end try
                    set recInfo to freqStr & ":::" & intVal & ":::" & untilStr & ":::" & cntStr
                end if
            end try

            set out to out & calName & delim & evUID & delim & evSummary & delim & evStartStr & delim & evEndStr & delim & evLoc & delim & evDesc & delim & evAllDay & delim & recInfo & linefeed
        end repeat
    end repeat
    return out
end tell

on fmtDate(d)
    set y to year of d
    set m to month of d as integer
    set dd to day of d
    set h to hours of d
    set mn to minutes of d
    set s to seconds of d
    set pad to "0"
    set mStr to (m as string)
    if m < 10 then set mStr to pad & mStr
    set dStr to (dd as string)
    if dd < 10 then set dStr to pad & dStr
    set hStr to (h as string)
    if h < 10 then set hStr to pad & hStr
    set mnStr to (mn as string)
    if mn < 10 then set mnStr to pad & mnStr
    set sStr to (s as string)
    if s < 10 then set sStr to pad & sStr
    return (y as string) & "-" & mStr & "-" & dStr & " " & hStr & ":" & mnStr & ":" & sStr
end fmtDate
'''


def run_osascript(lookback_days, lookahead_days):
    """执行 osascript 读取 Calendar.app 事件，返回原始输出文本。"""
    script = build_applescript(lookback_days, lookahead_days)
    try:
        result = subprocess.run(
            ["osascript", "-e", script],
            capture_output=True,
            text=True,
            timeout=120,
        )
    except FileNotFoundError:
        print("[ERR] osascript 未找到（非 macOS 环境？）", file=sys.stderr)
        sys.exit(3)
    except subprocess.TimeoutExpired:
        print("[ERR] osascript 超时（120s）", file=sys.stderr)
        sys.exit(3)

    if result.returncode != 0:
        stderr = result.stderr or ""
        combined = stderr + "\n" + (result.stdout or "")
        if is_tcc_error(combined):
            print(f"[TCC_ERROR] osascript 无法访问 Calendar.app: {stderr.strip()}", file=sys.stderr)
            sys.exit(2)
        raise RuntimeError(f"osascript 执行失败 (rc={result.returncode}): {stderr.strip()}")

    return result.stdout or ""


def is_tcc_error(text):
    """检测 osascript TCC 自动化授权错误。"""
    low = text.lower()
    if "-10004" in low or "10004" in low:
        return True
    if "权限违例" in text or "权限错误" in text:
        return True
    if "not authorized" in low or "not authorised" in low:
        return True
    if "appleevent" in low and "calendar" in low:
        return True
    if "automation" in low and "not allowed" in low:
        return True
    return False


# ============ 事件解析 ============

def parse_events(raw_output):
    """解析 osascript 输出为事件列表。

    Returns: list of dict, 每个 dict 包含:
      calendar_name, uid, summary, start_dt, end_dt, location, description,
      all_day, rec_freq, rec_interval, rec_until, rec_count
    """
    events = []
    for line in raw_output.strip().split("\n"):
        line = line.strip()
        if not line:
            continue
        parts = line.split("|||")
        if len(parts) < 8:
            continue

        cal_name = parts[0].strip()
        uid = parts[1].strip()
        summary = parts[2].strip()
        start_str = parts[3].strip()
        end_str = parts[4].strip()
        location = parts[5].strip()
        description = parts[6].strip()
        all_day = parts[7].strip().lower() == "true"

        rec_info = parts[8].strip() if len(parts) > 8 else ""
        rec_freq, rec_interval, rec_until, rec_count = "", 1, "", 0
        if rec_info and ":::" in rec_info:
            rec_parts = rec_info.split(":::")
            rec_freq = rec_parts[0].strip().lower() if len(rec_parts) > 0 else ""
            rec_interval = int(rec_parts[1]) if len(rec_parts) > 1 and rec_parts[1].strip().isdigit() else 1
            rec_until = rec_parts[2].strip() if len(rec_parts) > 2 else ""
            rec_count = int(rec_parts[3]) if len(rec_parts) > 3 and rec_parts[3].strip().isdigit() else 0

        start_dt = parse_applescript_date(start_str)
        end_dt = parse_applescript_date(end_str)

        if start_dt is None:
            continue

        events.append({
            "calendar_name": cal_name,
            "uid": uid,
            "summary": summary,
            "start_dt": start_dt,
            "end_dt": end_dt,
            "location": location,
            "description": description,
            "all_day": all_day,
            "rec_freq": rec_freq,
            "rec_interval": rec_interval,
            "rec_until": rec_until,
            "rec_count": rec_count,
        })

    return events


def parse_applescript_date(s):
    """解析 AppleScript 格式化日期 'YYYY-MM-DD HH:MM:SS'。"""
    if not s:
        return None
    for fmt in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%d %H:%M", "%Y-%m-%d"):
        try:
            return datetime.strptime(s, fmt)
        except ValueError:
            continue
    return None


# ============ 循环事件展开 ============

def expand_recurring(event, window_start, window_end):
    """展开循环事件，返回 [(start_dt, end_dt), ...] 列表。

    如果事件无循环规则或 dateutil 不可用，返回 [(event.start_dt, event.end_dt)]。
    """
    freq_str = event.get("rec_freq", "")
    if not freq_str or not HAS_DATEUTIL:
        return [(event["start_dt"], event["end_dt"])]

    freq = FREQ_MAP.get(freq_str)
    if freq is None:
        return [(event["start_dt"], event["end_dt"])]

    interval = event.get("rec_interval", 1) or 1
    base_start = event["start_dt"]
    duration = None
    if event["end_dt"]:
        duration = event["end_dt"] - base_start

    # 构建 rrule 参数
    rrule_kwargs = {
        "freq": freq,
        "interval": interval,
        "dtstart": base_start,
    }

    # until 日期（仅当可解析为有效日期时才加入）
    if event.get("rec_until"):
        until_dt = parse_applescript_date(event["rec_until"])
        if until_dt:
            rrule_kwargs["until"] = until_dt

    # count
    if event.get("rec_count") and event["rec_count"] > 0:
        rrule_kwargs["count"] = event["rec_count"]

    try:
        rule = rrule(**rrule_kwargs)
        # 用 between() 限制展开范围，避免无限序列导致 list() 卡死
        # inc=True 包含边界点
        occurrences = list(rule.between(window_start, window_end, inc=True))
    except Exception:
        return [(base_start, event["end_dt"])]

    result = []
    for occ_start in occurrences:
        occ_end = occ_start + duration if duration else None
        result.append((occ_start, occ_end))

    return result if result else [(base_start, event["end_dt"])]


# ============ 数据库写入 ============

def ensure_columns(conn):
    """确保 calendar_events 表有 source/source_account/external_event_id 列。"""
    cursor = conn.cursor()
    for col, ddl in [
        ("source", "ALTER TABLE calendar_events ADD COLUMN source TEXT DEFAULT ''"),
        ("source_account", "ALTER TABLE calendar_events ADD COLUMN source_account TEXT DEFAULT ''"),
        ("external_event_id", "ALTER TABLE calendar_events ADD COLUMN external_event_id TEXT DEFAULT ''"),
    ]:
        try:
            cursor.execute(ddl)
        except sqlite3.OperationalError:
            pass  # 列已存在

    # 唯一索引（仅在不存在时创建）
    try:
        cursor.execute(
            "CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_events_external "
            "ON calendar_events(source_account, external_event_id) WHERE source_account != ''"
        )
    except sqlite3.OperationalError:
        pass
    conn.commit()


def upsert_event(conn, event, occurrence_start, occurrence_end):
    """插入事件（INSERT OR IGNORE 利用唯一索引去重）。

    Returns: True=新增成功, False=跳过（已存在）
    """
    source_account = event["calendar_name"] or "apple_calendar"
    uid = event["uid"]

    # 循环事件的每次发生用 uid@时间戳 作为唯一 ID
    if event.get("rec_freq"):
        ext_id = f"{uid}@{occurrence_start.strftime('%Y%m%dT%H%M%S')}"
    else:
        ext_id = uid

    date_str = occurrence_start.strftime("%Y-%m-%d")
    time_str = occurrence_start.strftime("%H:%M")
    start_iso = occurrence_start.strftime("%Y-%m-%d %H:%M:%S")
    end_iso = ""
    if occurrence_end:
        end_iso = occurrence_end.strftime("%Y-%m-%d %H:%M:%S")

    cursor = conn.cursor()
    try:
        cursor.execute(
            """INSERT OR IGNORE INTO calendar_events
               (member_id, title, description, start_time, end_time, date, time,
                location, source, source_account, external_event_id)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                DEFAULT_MEMBER_ID,
                event["summary"] or "(无标题)",
                event["description"],
                start_iso,
                end_iso,
                date_str,
                time_str,
                event["location"],
                SOURCE,
                source_account,
                ext_id,
            ),
        )
        affected = cursor.rowcount
        conn.commit()
        return affected > 0
    except sqlite3.IntegrityError:
        return False


# ============ 主流程 ============

def main():
    parser = argparse.ArgumentParser(description="HomeMate Apple 日历同步")
    parser.add_argument("--lookback", type=int, default=30, help="回看天数（默认 30）")
    parser.add_argument("--lookahead", type=int, default=30, help="前瞻天数（默认 30）")
    args = parser.parse_args()

    if not os.path.exists(DB_PATH):
        print(f"[ERR] 数据库不存在: {DB_PATH}", file=sys.stderr)
        print(f"成功: 0 | 跳过: 0 | 失败: 1")
        sys.exit(1)

    now = datetime.now()
    window_start = now - timedelta(days=args.lookback)
    window_end = now + timedelta(days=args.lookahead)

    # 1. 读取 Calendar.app 事件
    try:
        raw = run_osascript(args.lookback, args.lookahead)
    except SystemExit:
        raise  # TCC 错误等已处理
    except Exception as e:
        print(f"[ERR] 读取日历失败: {e}", file=sys.stderr)
        print(f"成功: 0 | 跳过: 0 | 失败: 1")
        sys.exit(1)

    events = parse_events(raw)
    if not events:
        print("[INFO] Calendar.app 中无 ±{} 天事件".format(args.lookback))
        print("成功: 0 | 跳过: 0 | 失败: 0")
        return

    if not HAS_DATEUTIL:
        print("[WARN] python-dateutil 未安装，循环事件不展开（pip3 install python-dateutil）", file=sys.stderr)

    # 2. 写入数据库
    conn = sqlite3.connect(DB_PATH)
    ensure_columns(conn)

    ok_count = 0
    skip_count = 0
    fail_count = 0

    for event in events:
        # 展开循环事件
        try:
            occurrences = expand_recurring(event, window_start, window_end)
        except Exception as e:
            print(f"[WARN] 展开循环事件失败 (uid={event['uid']}): {e}", file=sys.stderr)
            occurrences = [(event["start_dt"], event["end_dt"])]

        for occ_start, occ_end in occurrences:
            try:
                inserted = upsert_event(conn, event, occ_start, occ_end)
                if inserted:
                    ok_count += 1
                else:
                    skip_count += 1
            except Exception as e:
                print(f"[WARN] 写入事件失败 (uid={event['uid']}): {e}", file=sys.stderr)
                fail_count += 1

    conn.close()

    # 3. 输出计数（最后一行，供 scheduler 解析）
    print(f"[OK] Apple 日历同步完成: {ok_count} 新增, {skip_count} 跳过, {fail_count} 失败")
    print(f"成功: {ok_count} | 跳过: {skip_count} | 失败: {fail_count}")


if __name__ == "__main__":
    main()
