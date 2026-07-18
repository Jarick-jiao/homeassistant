package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// CreateCalendarEvent 创建日历事件（含扩展字段）
func (db *DB) CreateCalendarEvent(ctx context.Context, e *model.CalendarEvent) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO calendar_events (member_id, title, description, start_time, end_time, date, time, location, event_type, type, is_important, recurrence_rule, reminder_minutes, created_by, color)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.MemberID, e.Title, e.Description, e.StartTime, e.EndTime, e.Date, e.Time,
		e.Location, e.EventType, e.Type, boolToInt(e.IsImportant), e.RecurrenceRule,
		e.ReminderMinutes, e.CreatedBy, e.Color)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateCalendarEvent 更新事件
func (db *DB) UpdateCalendarEvent(ctx context.Context, e *model.CalendarEvent) error {
	res, err := db.conn.ExecContext(ctx,
		`UPDATE calendar_events SET title=?, description=?, start_time=?, end_time=?, date=?, time=?, location=?, event_type=?, type=?, is_important=?, recurrence_rule=?, reminder_minutes=?, color=? WHERE id=?`,
		e.Title, e.Description, e.StartTime, e.EndTime, e.Date, e.Time,
		e.Location, e.EventType, e.Type, boolToInt(e.IsImportant), e.RecurrenceRule,
		e.ReminderMinutes, e.Color, e.ID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("事件不存在")
	}
	return nil
}

// DeleteCalendarEvent 删除事件
func (db *DB) DeleteCalendarEvent(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, `DELETE FROM calendar_events WHERE id=?`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("事件不存在")
	}
	return nil
}

// GetCalendarEventByID 单个事件详情
func (db *DB) GetCalendarEventByID(ctx context.Context, id int64) (*model.CalendarEvent, error) {
	var e model.CalendarEvent
	var isImp int
	var startTime, endTime sql.NullTime
	var lastReminded sql.NullTime
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, member_id, title, description, start_time, end_time, date, time, location, event_type, type, is_important, recurrence_rule, reminder_minutes, last_reminded_at, created_by, color
		 FROM calendar_events WHERE id=?`, id).
		Scan(&e.ID, &e.MemberID, &e.Title, &e.Description, &startTime, &endTime, &e.Date, &e.Time,
			&e.Location, &e.EventType, &e.Type, &isImp, &e.RecurrenceRule, &e.ReminderMinutes,
			&lastReminded, &e.CreatedBy, &e.Color)
	if err != nil {
		return nil, err
	}
	e.IsImportant = isImp == 1
	if startTime.Valid {
		e.StartTime = startTime.Time
	}
	if endTime.Valid {
		e.EndTime = endTime.Time
	}
	if lastReminded.Valid {
		t := lastReminded.Time
		e.LastRemindedAt = &t
	}
	return &e, nil
}

// ListCalendarEventsByDateRange 按日期范围查事件
func (db *DB) ListCalendarEventsByDateRange(ctx context.Context, from, to string) ([]model.CalendarEvent, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_id, title, description, start_time, end_time, date, time, location, event_type, type, is_important, recurrence_rule, reminder_minutes, last_reminded_at, created_by, color
		 FROM calendar_events WHERE date >= ? AND date <= ? ORDER BY start_time ASC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.CalendarEvent
	for rows.Next() {
		var e model.CalendarEvent
		var isImp int
		var startTime, endTime, lastReminded sql.NullTime
		if err := rows.Scan(&e.ID, &e.MemberID, &e.Title, &e.Description, &startTime, &endTime, &e.Date, &e.Time,
			&e.Location, &e.EventType, &e.Type, &isImp, &e.RecurrenceRule, &e.ReminderMinutes,
			&lastReminded, &e.CreatedBy, &e.Color); err != nil {
			continue
		}
		e.IsImportant = isImp == 1
		if startTime.Valid {
			e.StartTime = startTime.Time
		}
		if endTime.Valid {
			e.EndTime = endTime.Time
		}
		if lastReminded.Valid {
			t := lastReminded.Time
			e.LastRemindedAt = &t
		}
		list = append(list, e)
	}
	return list, nil
}

// ListUpcomingEventsWithReminders 查询需要提醒的事件
func (db *DB) ListUpcomingEventsWithReminders(ctx context.Context, now time.Time, lookahead time.Duration) ([]model.CalendarEvent, error) {
	until := now.Add(lookahead)
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_id, title, description, start_time, end_time, date, time, location, event_type, type, is_important, recurrence_rule, reminder_minutes, last_reminded_at, created_by, color
		 FROM calendar_events WHERE start_time IS NOT NULL AND start_time <= ? AND start_time >= ?`,
		until, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.CalendarEvent
	for rows.Next() {
		var e model.CalendarEvent
		var isImp int
		var startTime, endTime, lastReminded sql.NullTime
		if err := rows.Scan(&e.ID, &e.MemberID, &e.Title, &e.Description, &startTime, &endTime, &e.Date, &e.Time,
			&e.Location, &e.EventType, &e.Type, &isImp, &e.RecurrenceRule, &e.ReminderMinutes,
			&lastReminded, &e.CreatedBy, &e.Color); err != nil {
			continue
		}
		e.IsImportant = isImp == 1
		if startTime.Valid {
			e.StartTime = startTime.Time
		}
		if endTime.Valid {
			e.EndTime = endTime.Time
		}
		if lastReminded.Valid {
			t := lastReminded.Time
			e.LastRemindedAt = &t
		}
		list = append(list, e)
	}
	return list, nil
}

// MarkEventReminded 标记事件已提醒
func (db *DB) MarkEventReminded(ctx context.Context, id int64, at time.Time) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE calendar_events SET last_reminded_at=? WHERE id=?`, at, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DeleteAllEvents 清空全部日历事件（管理员）
func (db *DB) DeleteAllEvents(ctx context.Context) (int64, error) {
	res, err := db.conn.ExecContext(ctx, "DELETE FROM calendar_events")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SeedDemoEvents 插入预设 Demo 日程（按 title 查重避免重复种子）
func (db *DB) SeedDemoEvents(ctx context.Context) (int, error) {
	now := time.Now()
	demos := []model.CalendarEvent{
		{Title: "小明生日", Description: "庆祝 10 岁生日", Date: now.AddDate(0, 1, 2).Format("2006-01-02"), Time: "18:00", Location: "家中", EventType: "birthday", Type: "family", IsImportant: true, ReminderMinutes: 1440, Color: "#ef4444"},
		{Title: "结婚纪念日", Description: "爸妈结婚 15 周年", Date: now.AddDate(0, 2, 5).Format("2006-01-02"), Time: "19:00", Location: "餐厅", EventType: "anniversary", Type: "family", IsImportant: true, ReminderMinutes: 2880, Color: "#ec4899"},
		{Title: "全家健康体检", Description: "年度体检，空腹", Date: now.AddDate(0, 0, 10).Format("2006-01-02"), Time: "08:30", Location: "市中心医院", EventType: "medical", Type: "health", IsImportant: true, ReminderMinutes: 720, Color: "#10b981"},
		{Title: "家长会", Description: "期中家长会", Date: now.AddDate(0, 0, 15).Format("2006-01-02"), Time: "15:00", Location: "学校三楼会议室", EventType: "school", Type: "edu", ReminderMinutes: 480, Color: "#3b82f6"},
		{Title: "家庭聚餐", Description: "周末全家聚餐", Date: now.AddDate(0, 0, 6).Format("2006-01-02"), Time: "12:00", Location: "外婆家", EventType: "gathering", Type: "family", ReminderMinutes: 240, Color: "#f59e0b"},
		{Title: "周末郊游", Description: "森林公园野餐", Date: now.AddDate(0, 0, 7).Format("2006-01-02"), Time: "09:00", Location: "森林公园西门", EventType: "outing", Type: "leisure", ReminderMinutes: 480, Color: "#84cc16"},
		{Title: "信用卡账单日", Description: "本月账单到期", Date: now.AddDate(0, 0, 20).Format("2006-01-02"), Time: "00:00", EventType: "finance", Type: "finance", ReminderMinutes: 1440, Color: "#6366f1"},
		{Title: "疫苗接种", Description: "弟弟流感疫苗第二针", Date: now.AddDate(0, 0, 25).Format("2006-01-02"), Time: "10:00", Location: "社区卫生服务中心", EventType: "medical", Type: "health", ReminderMinutes: 720, Color: "#10b981"},
	}
	count := 0
	for _, e := range demos {
		// 按 title 查重
		var existingID int64
		_ = db.conn.QueryRowContext(ctx, "SELECT id FROM calendar_events WHERE title=? LIMIT 1", e.Title).Scan(&existingID)
		if existingID > 0 {
			continue
		}
		startTime, _ := time.Parse("2006-01-02 15:04", e.Date+" "+e.Time)
		if startTime.IsZero() {
			startTime = now
		}
		e.StartTime = startTime
		e.EndTime = startTime.Add(2 * time.Hour)
		if _, err := db.CreateCalendarEvent(ctx, &e); err != nil {
			continue
		}
		count++
	}
	return count, nil
}
