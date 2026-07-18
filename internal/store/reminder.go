package store

import (
	"context"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// ListRemindersByMember 列出成员提醒（status 可选 "","pending","sent"）
func (db *DB) ListRemindersByMember(ctx context.Context, memberID int64, status string) ([]model.Reminder, error) {
	query := "SELECT id, member_id, title, content, remind_at, status, channel FROM reminders WHERE member_id=?"
	args := []interface{}{memberID}
	if status != "" {
		query += " AND status=?"
		args = append(args, status)
	}
	query += " ORDER BY remind_at ASC"
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Reminder
	for rows.Next() {
		var r model.Reminder
		if err := rows.Scan(&r.ID, &r.MemberID, &r.Title, &r.Content, &r.RemindAt, &r.Status, &r.Channel); err != nil {
			continue
		}
		list = append(list, r)
	}
	return list, nil
}

// UpdateReminder 更新提醒
func (db *DB) UpdateReminder(ctx context.Context, r *model.Reminder) error {
	res, err := db.conn.ExecContext(ctx,
		`UPDATE reminders SET title=?, content=?, remind_at=?, channel=? WHERE id=?`,
		r.Title, r.Content, r.RemindAt, r.Channel, r.ID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("提醒不存在")
	}
	return nil
}

// DeleteReminder 删除提醒
func (db *DB) DeleteReminder(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, `DELETE FROM reminders WHERE id=?`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("提醒不存在")
	}
	return nil
}

// ListDueReminders 查询到期提醒
func (db *DB) ListDueReminders(ctx context.Context, now time.Time) ([]model.Reminder, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_id, title, content, remind_at, status, channel
		 FROM reminders WHERE status='pending' AND remind_at <= ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Reminder
	for rows.Next() {
		var r model.Reminder
		if err := rows.Scan(&r.ID, &r.MemberID, &r.Title, &r.Content, &r.RemindAt, &r.Status, &r.Channel); err != nil {
			continue
		}
		list = append(list, r)
	}
	return list, nil
}

// MarkReminderSent 标记提醒已发送
func (db *DB) MarkReminderSent(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `UPDATE reminders SET status='sent' WHERE id=?`, id)
	return err
}
