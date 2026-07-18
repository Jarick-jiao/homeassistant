package store

import (
	"context"
	"time"
)

// DeleteNotificationsBefore 删除指定时间前的已读通知
func (db *DB) DeleteNotificationsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"DELETE FROM notifications WHERE created_at < ? AND read_at IS NOT NULL", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeletePointsRecordsBefore 删除指定时间前的积分记录
func (db *DB) DeletePointsRecordsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"DELETE FROM points_records WHERE created_at < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteChatMessagesBefore 删除指定时间前的聊天消息
// chat_messages 表的时间列是 `timestamp`（INTEGER Unix 秒），无 created_at 列
func (db *DB) DeleteChatMessagesBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"DELETE FROM chat_messages WHERE timestamp < ?", before.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteMessagesBefore 删除指定时间前的已读留言板消息
func (db *DB) DeleteMessagesBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"DELETE FROM message_board WHERE created_at < ? AND read_at IS NOT NULL", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteDeviceDataBefore 删除指定时间前的设备数据
// device_data 表的时间列是 `received_at`，无 created_at 列
func (db *DB) DeleteDeviceDataBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"DELETE FROM device_data WHERE received_at < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
