package store

import (
	"context"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// CreateNotification 创建通知
func (db *DB) CreateNotification(ctx context.Context, n *model.Notification) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO notifications (member_id, type, title, body, data_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.MemberID, n.Type, n.Title, n.Body, n.DataJSON, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListNotifications 列出成员通知（分页）
func (db *DB) ListNotifications(ctx context.Context, memberID int64, limit, offset int) ([]model.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_id, type, title, body, data_json, read_at, pushed_at, created_at
		 FROM notifications WHERE member_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		memberID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.MemberID, &n.Type, &n.Title, &n.Body, &n.DataJSON, &n.ReadAt, &n.PushedAt, &n.CreatedAt); err != nil {
			continue
		}
		list = append(list, n)
	}
	return list, nil
}

// UnreadNotificationCount 未读数量
func (db *DB) UnreadNotificationCount(ctx context.Context, memberID int64) (int, error) {
	var count int
	err := db.conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE member_id=? AND read_at IS NULL`, memberID).Scan(&count)
	return count, err
}

// MarkNotificationRead 标记已读
func (db *DB) MarkNotificationRead(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE notifications SET read_at=? WHERE id=? AND read_at IS NULL`, time.Now(), id)
	return err
}

// MarkAllNotificationsRead 全部已读
func (db *DB) MarkAllNotificationsRead(ctx context.Context, memberID int64) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE notifications SET read_at=? WHERE member_id=? AND read_at IS NULL`, time.Now(), memberID)
	return err
}

// ListUnpushedNotifications 查询未推送通知（用于 webhook 推送）
func (db *DB) ListUnpushedNotifications(ctx context.Context, limit int) ([]model.Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_id, type, title, body, data_json, read_at, pushed_at, created_at
		 FROM notifications WHERE pushed_at IS NULL ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.MemberID, &n.Type, &n.Title, &n.Body, &n.DataJSON, &n.ReadAt, &n.PushedAt, &n.CreatedAt); err != nil {
			continue
		}
		list = append(list, n)
	}
	return list, nil
}

// MarkNotificationPushed 标记已推送
func (db *DB) MarkNotificationPushed(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE notifications SET pushed_at=? WHERE id=?`, time.Now(), id)
	return err
}

// MarkNotificationFailed 标记推送失败
func (db *DB) MarkNotificationFailed(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE notifications SET pushed_at=? WHERE id=?`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("标记失败: %w", err)
	}
	return nil
}
