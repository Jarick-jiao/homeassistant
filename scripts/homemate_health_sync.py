#!/usr/bin/env python3
"""
HomeMate 健康数据自动同步脚本 (v3.9.0 全字段版)
从 Garmin Connect 获取家庭成员健康数据，直接写入 SQLite health_data_cache 表。

不依赖 HTTP 服务，绕过 Go 客户端遇到的 Cloudflare 拦截（garminconnect 内部基于 garth，
TLS 指纹更接近真实浏览器）。

数据源配置从 data_source_config 表读取:
  - user_id    = Garmin 用户名/邮箱
  - api_secret = Garmin 密码
也支持环境变量 GARMIN_USERNAME / GARMIN_PASSWORD 兜底。

字段映射依据 internal/model/models.go 的 HealthDataCache 注释（Garmin API key 一一对齐），
覆盖全部 30 个字段（A 类基础指标 + B 类扩展指标），写入列顺序与 Go 端
UpsertHealthDataCache 完全一致。

用法:
  python3 homemate_health_sync.py                       # 同步今天（从 DB 读凭证）
  python3 homemate_health_sync.py --member 2             # 指定成员
  python3 homemate_health_sync.py --date 2026-07-18      # 指定日期
  python3 homemate_health_sync.py --backfill 7          # 回填最近 7 天
  python3 homemate_health_sync.py --list                 # 查看缓存数据
  python3 homemate_health_sync.py --config               # 查看数据源配置
  python3 homemate_health_sync.py --save-config --member 2 --name 爸爸 \\
         --username <user> --password <pass>            # 配置凭证

数据库: /Users/jarick/workspace/codebook/homework/homeassistant/homemate.db
运行:   cd /Users/jarick/.trae-cn/work/6a5b3a84cceb40950b6e7492 && \\
        env -u PYTHONHOME -u PYTHONPATH /usr/bin/python3 homemate_health_sync.py
"""

import argparse
import json
import os
import sqlite3
import sys
from datetime import datetime, timedelta

# ============ 配置 ============

DB_PATH = os.environ.get(
    "HOMEMATE_DB",
    "/Users/jarick/workspace/codebook/homework/homeassistant/homemate.db",
)
# Garmin token 缓存目录（按用户隔离，避免多成员串号）
TOKENSTORE_DIR = os.environ.get(
    "GARMIN_TOKENSTORE_DIR", os.path.expanduser("~/.garminconnect_tokens")
)
# 是否中国区 Garmin Connect（garmin.com.cn）
IS_CN = os.environ.get("GARMIN_IS_CN", "1") not in ("0", "false", "False")
SOURCE = "garmin"


# ============ 通用工具 ============

def dig(obj, *keys, default=None):
    """安全读取多层嵌套 dict，任一层缺失或类型不符返回 default。

    支持 list 索引：dig(data, "list", 0, "key")
    """
    cur = obj
    for k in keys:
        if cur is None:
            return default
        if isinstance(k, int):
            if isinstance(cur, list) and 0 <= k < len(cur):
                cur = cur[k]
            else:
                return default
        else:
            if isinstance(cur, dict):
                cur = cur.get(k)
            else:
                return default
    return cur if cur is not None else default


def to_int(val, default=0):
    """转 int，None/空串/非数字返回 default"""
    if val is None or val == "":
        return default
    try:
        return int(round(float(val)))
    except (TypeError, ValueError):
        return default


def to_float(val, default=0.0):
    """转 float"""
    if val is None or val == "":
        return default
    try:
        return float(val)
    except (TypeError, ValueError):
        return default


def to_str(val, default=""):
    if val is None:
        return default
    return str(val)


def hours_from_seconds(sec, ndigits=2):
    """秒 -> 小时"""
    s = to_float(sec, 0)
    return round(s / 3600.0, ndigits) if s > 0 else 0.0


# ============ 数据库凭证 ============

def get_garmin_credentials(conn, member_id=None):
    """从 data_source_config 表读取 Garmin 凭证 (is_active=1)。

    Returns: [(member_id, member_name, username, password, is_active)]
    """
    cursor = conn.cursor()
    if member_id:
        cursor.execute(
            "SELECT member_id, member_name, user_id, api_secret, is_active "
            "FROM data_source_config WHERE member_id=? AND source_type='garmin' AND is_active=1",
            (member_id,),
        )
    else:
        cursor.execute(
            "SELECT member_id, member_name, user_id, api_secret, is_active "
            "FROM data_source_config WHERE source_type='garmin' AND is_active=1"
        )
    rows = cursor.fetchall()
    if not rows:
        env_user = os.environ.get("GARMIN_USERNAME", "")
        env_pass = os.environ.get("GARMIN_PASSWORD", "")
        if env_user and env_pass:
            print("[INFO] 未在 DB 找到配置，使用环境变量 GARMIN_USERNAME/GARMIN_PASSWORD")
            return [(member_id or 1, "环境变量配置", env_user, env_pass, 1)]
        return []
    return [(r[0], r[1], r[2], r[3], r[4]) for r in rows]


def save_config(conn, member_id, member_name, username, password):
    """保存 Garmin 数据源配置（member_id 无 UNIQUE 约束，手动 upsert）"""
    cursor = conn.cursor()
    cursor.execute(
        "SELECT id FROM data_source_config WHERE member_id=? AND source_type='garmin'",
        (member_id,),
    )
    existing = cursor.fetchone()
    if existing:
        cursor.execute(
            """UPDATE data_source_config SET member_name=?, user_id=?, api_secret=?, is_active=1
               WHERE member_id=? AND source_type='garmin'""",
            (member_name, username, password, member_id),
        )
    else:
        cursor.execute(
            """INSERT INTO data_source_config
               (member_id, member_name, source_type, user_id, api_secret, is_active, created_at)
               VALUES (?, ?, 'garmin', ?, ?, 1, datetime('now'))""",
            (member_id, member_name, username, password),
        )
    conn.commit()
    print(f"[OK] 数据源配置已保存: member_id={member_id}, name={member_name}, user={username}")


def update_last_sync(conn, member_ids):
    """更新 data_source_config.last_sync_at"""
    cursor = conn.cursor()
    now = datetime.now().isoformat()
    for mid in member_ids:
        cursor.execute(
            "UPDATE data_source_config SET last_sync_at=? WHERE member_id=? AND source_type='garmin'",
            (now, mid),
        )
    conn.commit()


# ============ Garmin 登录 ============

def login_garmin(username, password, member_id=None):
    """登录 Garmin Connect（中国区 + tokenstore 缓存）。

    v3.9.11: 适配 garminconnect >= 0.3.0 新认证流程。
    背景：2026-03 Garmin 改了认证基础设施，garth 库已废弃，
    garminconnect 0.3.x 起改用 curl_cffi + ua-generator 的 Mobile SSO 流程，
    login() API 签名变更（不再返回 tuple，改用 prompt_mfa 回调）。
    旧 token 格式不兼容，首次升级后需清除 ~/.garminconnect_tokens 重新登录。

    Returns: (client, None) 成功 | (None, err_str) 失败
    """
    try:
        from garminconnect import Garmin
    except ImportError:
        print("[ERR] garminconnect 库未安装。请运行: pip3 install 'garminconnect>=0.3.0' --break-system-packages",
              file=sys.stderr)
        sys.exit(1)

    try:
        import garminconnect
        gv = getattr(garminconnect, "__version__", "0")
        if gv < "0.3.0":
            print(f"[WARN] garminconnect {gv} < 0.3.0，2026-03 后 Garmin 认证 API 已变更，"
                  f"旧版 garth 依赖已失效。请升级: pip3 install --upgrade garminconnect",
                  file=sys.stderr)
    except Exception:
        pass

    # 每个成员独立 tokenstore，避免多账号串号
    tokenstore = TOKENSTORE_DIR
    if member_id is not None:
        tokenstore = os.path.join(TOKENSTORE_DIR, f"member_{member_id}")
    os.makedirs(tokenstore, exist_ok=True)

    try:
        # v3.9.11: 新 API — prompt_mfa 回调替代 return_on_mfa
        # 无 MFA 账号传 None 即可；有 MFA 账号会触发回调（脚本不支持交互，
        # 会抛异常提示用户用 --save-config 配置无 MFA 账号或手动登录一次缓存 token）。
        client = Garmin(username, password, prompt_mfa=None, is_cn=IS_CN)
        client.login(tokenstore)
        try:
            client.garth.dump(tokenstore)
            print(f"  [OK] token 已缓存: {tokenstore}")
        except Exception:
            pass
        return client, None
    except Exception as e:
        # 详细错误信息打到 stderr，scheduler 会捕获并记录到日志
        import traceback
        err_msg = f"{type(e).__name__}: {e}"
        print(f"[ERR] Garmin 登录失败 (member={member_id}): {e}", file=sys.stderr)
        print(f"[ERR] 错误类型: {type(e).__name__}", file=sys.stderr)
        # 429 限流提示（clientId+账号绑定，VPN 无效，需静置 24h）
        err_str = str(e).lower()
        if "429" in err_str or "too many" in err_str or "rate" in err_str:
            err_msg = "触发 Garmin SSO 限流(429)：与 clientId+账号绑定，VPN/换网无效，需停止所有登录尝试静置 24 小时后重试（浏览器登录不受影响）"
            print(f"[ERR] {err_msg}", file=sys.stderr)
        # 旧 token 不兼容提示
        elif "token" in err_str or "oauth" in err_str or "auth" in err_str:
            err_msg = f"可能是旧 token 不兼容，请删除 {tokenstore} 后重试"
            print(f"[ERR] {err_msg}", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        return None, err_msg


# ============ Garmin 数据获取 ============

def fetch_garmin_data(client, date_str):
    """聚合多个 Garmin 端点，返回完整 HealthDataCache 字段 dict。

    端点:
      - get_user_summary  : 步数/心率/压力/身体电量/卡路里/距离/步数目标/体重/血压
      - get_sleep_data    : 睡眠时长/深睡/REM/睡眠评分
      - get_hrv_data      : 平均夜间 HRV
      - get_respiration_data : 平均清醒呼吸
    """
    summary = {}
    sleep = {}
    hrv = {}
    respiration = {}

    # 1. 用户摘要（wellness dailySummary，主数据源）
    try:
        summary = client.get_user_summary(date_str) or {}
    except Exception as e:
        print(f"  [WARN] get_user_summary 失败: {e}", file=sys.stderr)

    # 2. 睡眠数据（dailySleepDTO，睡眠评分/深睡/REM 的权威来源）
    try:
        sleep = client.get_sleep_data(date_str) or {}
    except Exception as e:
        print(f"  [WARN] get_sleep_data 失败: {e}", file=sys.stderr)

    # 3. HRV
    try:
        hrv = client.get_hrv_data(date_str) or {}
    except Exception as e:
        print(f"  [WARN] get_hrv_data 失败: {e}", file=sys.stderr)

    # 4. 呼吸
    try:
        respiration = client.get_respiration_data(date_str) or {}
    except Exception as e:
        print(f"  [WARN] get_respiration_data 失败: {e}", file=sys.stderr)

    return map_garmin_to_cache(summary, sleep, hrv, respiration)


def map_garmin_to_cache(summary, sleep, hrv, respiration):
    """将 Garmin 多端点响应映射到 HealthDataCache 全部 30 字段。

    字段映射权威来源: internal/model/models.go HealthDataCache 注释。
    sleep 字段优先取 sleep 端点的 dailySleepDTO，回退 summary 中的同名字段。
    """
    # --- sleep 端点: dailySleepDTO 可能直接在 sleep 顶层，也可能嵌在 summary 内 ---
    daily_sleep = dig(sleep, "dailySleepDTO") or dig(summary, "dailySleepDTO") or {}
    sleep_scores = dig(daily_sleep, "sleepScores") or {}
    sleep_overall = dig(sleep_scores, "overall") or {}

    # --- HRV: hrvSummary.avgOvernightHrv ---
    hrv_summary = dig(hrv, "hrvSummary") or {}
    # 兼容部分响应直接平铺 avgOvernightHrv
    avg_overnight_hrv = dig(hrv_summary, "avgOvernightHrv") or dig(hrv, "avgOvernightHrv")

    # --- respiration: respirationSummaryValues.avgWakingRespirationValue ---
    resp_values = dig(respiration, "respirationSummaryValues") or {}
    avg_waking_resp = dig(resp_values, "avgWakingRespirationValue") or dig(
        respiration, "avgWakingRespirationValue"
    )

    # --- 血压: measurementSummaries[0] ---
    bp_summaries = dig(summary, "measurementSummaries") or []
    bp_first = bp_summaries[0] if bp_summaries else {}

    # --- 体重: totalAverage.weight ---
    total_average = dig(summary, "totalAverage") or {}

    # --- 心率: maxHeartRate 作为展示心率 (model 注释明确) ---
    max_hr = to_int(dig(summary, "maxHeartRate"))
    min_hr = to_int(dig(summary, "minHeartRate"))

    return {
        # --- A 类基础指标 ---
        "steps": to_int(dig(summary, "totalSteps")),
        "heart_rate": max_hr,  # Garmin: maxHeartRate（当日最高心率作为展示心率）
        "sleep_hours": hours_from_seconds(dig(daily_sleep, "sleepTimeSeconds")),
        "sleep_score": to_int(dig(sleep_overall, "value")),  # dailySleepDTO.sleepScores.overall.value
        "blood_pressure_sys": to_int(dig(bp_first, "systolic")),
        "blood_pressure_dia": to_int(dig(bp_first, "diastolic")),
        "weight": to_float(dig(total_average, "weight")),
        "height": to_float(dig(summary, "height")),  # 用户档案（非每日 API，常为 0）
        "stress": to_int(dig(summary, "averageStressLevel")),
        "spo2": to_int(dig(summary, "latestSpo2") or dig(summary, "averageSpO2")),
        "body_battery": to_int(
            dig(summary, "bodyBatteryMostRecentValue")
            or dig(summary, "averageBodyBatteryChargedValue")
        ),
        "calories": to_int(dig(summary, "totalKilocalories")),
        # --- B1 睡眠详情 ---
        "deep_sleep_hours": hours_from_seconds(dig(daily_sleep, "deepSleepSeconds")),
        "rem_sleep_hours": hours_from_seconds(dig(daily_sleep, "remSleepSeconds")),
        # --- B2 心率详情 ---
        "min_heart_rate": min_hr,
        "max_heart_rate": max_hr,
        "resting_hr_7d_avg": to_int(dig(summary, "lastSevenDaysAvgRestingHeartRate")),
        # --- B3 压力详情 ---
        "max_stress": to_int(dig(summary, "maxStressLevel")),
        "stress_qualifier": to_str(dig(summary, "stressQualifier")),
        # --- B5 身体电量详情 ---
        "body_battery_highest": to_int(
            dig(summary, "bodyBatteryHighestValue")
            or dig(summary, "bodyBatteryChargedValue")
        ),
        "body_battery_lowest": to_int(dig(summary, "bodyBatteryLowestValue")),
        # --- B7 活动详情 ---
        "total_distance_m": to_int(dig(summary, "totalDistanceMeters")),
        "daily_step_goal": to_int(dig(summary, "dailyStepGoal")),
        # --- B8 呼吸 ---
        "avg_respiration": to_float(avg_waking_resp),
        # --- B9 HRV ---
        "avg_overnight_hrv": to_float(avg_overnight_hrv),
    }


# ============ 数据库写入 ============

# 列顺序与 Go UpsertHealthDataCache 完全一致
CACHE_COLUMNS = [
    "member_id", "date",
    "steps", "heart_rate", "sleep_hours", "sleep_score",
    "blood_pressure_sys", "blood_pressure_dia", "weight", "height",
    "stress", "spo2", "body_battery", "calories",
    "deep_sleep_hours", "rem_sleep_hours",
    "min_heart_rate", "max_heart_rate", "resting_hr_7d_avg",
    "max_stress", "stress_qualifier",
    "body_battery_highest", "body_battery_lowest",
    "total_distance_m", "daily_step_goal",
    "avg_respiration", "avg_overnight_hrv",
    "source", "synced_at",
]

# ON CONFLICT 时更新的列（排除 member_id, date 主键）
CACHE_UPDATE_COLS = [c for c in CACHE_COLUMNS if c not in ("member_id", "date")]


def upsert_health_cache(conn, member_id, date_str, health, source=SOURCE):
    """写入 health_data_cache 表 (upsert by member_id+date)。

    列顺序与 Go 端 internal/store/sqlite.go UpsertHealthDataCache 对齐。
    """
    cursor = conn.cursor()
    synced_at = datetime.now().isoformat()

    row = [member_id, date_str]
    row += [health.get(c, 0) for c in CACHE_UPDATE_COLS if c not in ("source", "synced_at")]
    row += [source, synced_at]

    placeholders = ", ".join(["?"] * len(CACHE_COLUMNS))
    col_list = ", ".join(CACHE_COLUMNS)
    update_clause = ", ".join(f"{c}=excluded.{c}" for c in CACHE_UPDATE_COLS)

    sql = (
        f"INSERT INTO health_data_cache ({col_list}) VALUES ({placeholders}) "
        f"ON CONFLICT(member_id, date) DO UPDATE SET {update_clause}"
    )
    cursor.execute(sql, row)
    conn.commit()


def check_already_synced(conn, member_id, date_str):
    """检查指定日期是否已有 garmin 源且步数>0 的记录（避免重复同步）"""
    cursor = conn.cursor()
    cursor.execute(
        "SELECT steps, source FROM health_data_cache WHERE member_id=? AND date=?",
        (member_id, date_str),
    )
    row = cursor.fetchone()
    if row and row[1] == SOURCE and (row[0] or 0) > 0:
        return True
    return False


# ============ 同步主流程 ============

def sync_member(conn, member_id, member_name, username, password, date_str, force=False):
    """同步单个成员的单日数据。Returns: 1=成功 0=跳过 -1=失败"""
    print(f"\n--- 成员: {member_name} (id={member_id}) 日期: {date_str} ---")

    if not username or not password:
        print("  [SKIP] 凭证不完整")
        return -1

    if not force and check_already_synced(conn, member_id, date_str):
        print("  [SKIP] 当日已有 garmin 数据，跳过（--force 可强制覆盖）")
        return 0

    client, err = login_garmin(username, password, member_id=member_id)
    if not client:
        print(f"  [FAIL] 登录失败: {err}")
        return -1

    health = fetch_garmin_data(client, date_str)
    if not health:
        print("  [FAIL] 获取数据失败")
        return -1

    # 简要输出关键指标
    print(
        f"  步数={health['steps']} 心率={health['heart_rate']} "
        f"睡眠={health['sleep_hours']}h 评分={health['sleep_score']} "
        f"血氧={health['spo2']}% 压力={health['stress']} "
        f"电量={health['body_battery']} HRV={health['avg_overnight_hrv']}"
    )

    upsert_health_cache(conn, member_id, date_str, health)
    print("  [OK] 已写入 health_data_cache")
    return 1


def run_sync(args):
    """执行同步"""
    if not os.path.exists(DB_PATH):
        print(f"[ERR] 数据库不存在: {DB_PATH}", file=sys.stderr)
        sys.exit(1)

    conn = sqlite3.connect(DB_PATH)

    # 计算待同步日期列表
    if args.backfill and args.backfill > 0:
        today = datetime.now()
        dates = [(today - timedelta(days=i)).strftime("%Y-%m-%d") for i in range(args.backfill)]
    else:
        dates = [args.date or datetime.now().strftime("%Y-%m-%d")]

    print(f"[SYNC] 数据库: {DB_PATH}")
    print(f"[SYNC] 日期: {dates}")
    print(f"[SYNC] 中国区: {IS_CN}  tokenstore: {TOKENSTORE_DIR}")

    total_success = 0
    total_skip = 0
    total_fail = 0
    synced_members = set()

    for date_str in dates:
        print(f"\n===== 日期 {date_str} =====")
        credentials = get_garmin_credentials(conn, args.member)
        if not credentials:
            print("[ERR] 未找到 Garmin 数据源配置", file=sys.stderr)
            print("  配置: python3 homemate_health_sync.py --save-config --member 2 "
                  "--name 爸爸 --username <user> --password <pass>", file=sys.stderr)
            print("  或设置环境变量 GARMIN_USERNAME / GARMIN_PASSWORD", file=sys.stderr)
            conn.close()
            sys.exit(1)

        print(f"找到 {len(credentials)} 个成员的 Garmin 配置")
        for member_id, member_name, username, password, _ in credentials:
            result = sync_member(
                conn, member_id, member_name, username, password, date_str,
                force=args.force,
            )
            if result == 1:
                total_success += 1
                synced_members.add(member_id)
            elif result == 0:
                total_skip += 1
            else:
                total_fail += 1

    # 更新 last_sync_at
    update_last_sync(conn, list(synced_members))

    print(f"\n===== 同步完成 =====")
    print(f"成功: {total_success} | 跳过: {total_skip} | 失败: {total_fail}")
    conn.close()


# ============ 查看命令 ============

def cmd_list(conn):
    cursor = conn.cursor()
    cursor.execute(
        "SELECT member_id, date, steps, heart_rate, sleep_hours, sleep_score, spo2, "
        "stress, body_battery, avg_overnight_hrv, source, synced_at "
        "FROM health_data_cache ORDER BY date DESC, member_id LIMIT 30"
    )
    rows = cursor.fetchall()
    print("=== health_data_cache (最近 30 条) ===")
    if not rows:
        print("  (空)")
        return
    for r in rows:
        print(
            f"  member={r[0]} date={r[1]} steps={r[2]} hr={r[3]} sleep={r[4]}h "
            f"score={r[5]} spo2={r[6]}% stress={r[7]} batt={r[8]} hrv={r[9]} src={r[10]}"
        )


def cmd_config(conn):
    cursor = conn.cursor()
    cursor.execute(
        "SELECT member_id, member_name, source_type, user_id, is_active, last_sync_at "
        "FROM data_source_config"
    )
    rows = cursor.fetchall()
    if not rows:
        print("暂无数据源配置。")
        print("配置: python3 homemate_health_sync.py --save-config --member 2 "
              "--name 爸爸 --username <user> --password <pass>")
        return
    print("=== 数据源配置 ===")
    for r in rows:
        print(
            f"  member_id={r[0]} name={r[1]} type={r[2]} user={r[3]} "
            f"active={r[4]} last_sync={r[5]}"
        )


# ============ CLI ============

def main():
    parser = argparse.ArgumentParser(
        description="HomeMate 健康数据同步脚本 (Garmin -> SQLite, v3.9.0 全字段)"
    )
    parser.add_argument("--member", type=int, help="指定成员 ID")
    parser.add_argument("--date", help="指定日期 (YYYY-MM-DD)，默认今天")
    parser.add_argument("--backfill", type=int, help="回填最近 N 天数据")
    parser.add_argument("--force", action="store_true", help="强制覆盖当日已有数据")
    parser.add_argument("--list", action="store_true", help="查看 health_data_cache 缓存")
    parser.add_argument("--config", action="store_true", help="查看数据源配置")
    parser.add_argument("--save-config", action="store_true", help="配置 Garmin 凭证")
    parser.add_argument("--username", help="Garmin 用户名/邮箱（配合 --save-config）")
    parser.add_argument("--password", help="Garmin 密码（配合 --save-config）")
    parser.add_argument("--name", help="成员名称（配置时使用）")
    args = parser.parse_args()

    if not os.path.exists(DB_PATH):
        print(f"[ERR] 数据库不存在: {DB_PATH}", file=sys.stderr)
        print(f"  可通过环境变量 HOMEMATE_DB 指定路径", file=sys.stderr)
        sys.exit(1)

    conn = sqlite3.connect(DB_PATH)
    try:
        if args.list:
            cmd_list(conn)
            return
        if args.config:
            cmd_config(conn)
            return
        if args.save_config:
            if not args.member or not args.username or not args.password:
                print("[ERR] 需要 --member, --username, --password", file=sys.stderr)
                sys.exit(1)
            save_config(conn, args.member, args.name or "成员", args.username, args.password)
            return
        run_sync(args)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
