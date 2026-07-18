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
