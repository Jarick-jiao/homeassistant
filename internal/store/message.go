package store

import (
	"context"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// CreateMessage 创建留言
func (db *DB) CreateMessage(ctx context.Context, m *model.MessageBoard) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO message_board (from_member_id, to_member_id, content, parent_id, pinned, created_at)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		m.FromMemberID, m.ToMemberID, m.Content, m.ParentID, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListMessages 列出留言（广播 + @我）
func (db *DB) ListMessages(ctx context.Context, memberID int64) ([]model.MessageBoard, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, from_member_id, to_member_id, content, parent_id, pinned, read_at, created_at
		 FROM message_board
		 WHERE to_member_id IS NULL OR to_member_id = ?
		 ORDER BY pinned DESC, created_at DESC LIMIT 200`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []model.MessageBoard
	for rows.Next() {
		var m model.MessageBoard
		var toID, parentID *int64
		var readAt *time.Time
		if err := rows.Scan(&m.ID, &m.FromMemberID, &toID, &m.Content, &parentID, &m.Pinned, &readAt, &m.CreatedAt); err != nil {
			continue
		}
		m.ToMemberID = toID
		m.ParentID = parentID
		m.ReadAt = readAt
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// GetMessage 获取单条留言
func (db *DB) GetMessage(ctx context.Context, id int64) (*model.MessageBoard, error) {
	var m model.MessageBoard
	var toID, parentID *int64
	var readAt *time.Time
	err := db.conn.QueryRowContext(ctx,
		`SELECT id, from_member_id, to_member_id, content, parent_id, pinned, read_at, created_at
		 FROM message_board WHERE id=?`, id).
		Scan(&m.ID, &m.FromMemberID, &toID, &m.Content, &parentID, &m.Pinned, &readAt, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.ToMemberID = toID
	m.ParentID = parentID
	m.ReadAt = readAt
	return &m, nil
}

// MarkMessageRead 标记留言已读
func (db *DB) MarkMessageRead(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx,
		`UPDATE message_board SET read_at=? WHERE id=? AND read_at IS NULL`, time.Now(), id)
	return err
}

// PinMessage 置顶/取消置顶
func (db *DB) PinMessage(ctx context.Context, id int64, pinned bool) error {
	pv := 0
	if pinned {
		pv = 1
	}
	res, err := db.conn.ExecContext(ctx, `UPDATE message_board SET pinned=? WHERE id=?`, pv, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("留言不存在")
	}
	return nil
}

// DeleteMessage 删除留言
func (db *DB) DeleteMessage(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, `DELETE FROM message_board WHERE id=?`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("留言不存在")
	}
	return nil
}

// GetMemberNameByID 通过成员 ID 查姓名
func (db *DB) GetMemberNameByID(ctx context.Context, memberID int64) string {
	var name string
	err := db.conn.QueryRowContext(ctx,
		`SELECT name FROM family_members WHERE id=?`, memberID).Scan(&name)
	if err != nil {
		return ""
	}
	return name
}

// GetMemberIDByUserID 通过 user_id 查 member_id
func (db *DB) GetMemberIDByUserID(ctx context.Context, userID int64) (int64, error) {
	var memberID int64
	err := db.conn.QueryRowContext(ctx,
		`SELECT id FROM family_members WHERE user_id=?`, userID).Scan(&memberID)
	return memberID, err
}
