package model

import "time"

// MessageBoard 家庭留言板
type MessageBoard struct {
	ID           int64      `json:"id"`
	FromMemberID int64      `json:"from_member_id"`
	FromName     string     `json:"from_name,omitempty"`
	ToMemberID   *int64     `json:"to_member_id,omitempty"`
	ToName       string     `json:"to_name,omitempty"`
	Content      string     `json:"content"`
	ParentID     *int64     `json:"parent_id,omitempty"`
	Pinned       bool       `json:"pinned"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ToView 脱敏视图（补齐发送人/接收人姓名）
func (m *MessageBoard) ToView(fromName, toName string) *MessageBoard {
	view := *m
	view.FromName = fromName
	view.ToName = toName
	return &view
}
