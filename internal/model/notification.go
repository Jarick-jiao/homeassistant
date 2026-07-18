package model

import "time"

// 通知类型常量
const (
	NotificationTypeMessage  = "message"
	NotificationTypeReminder = "reminder"
	NotificationTypeChore     = "chore"
	NotificationTypeWeekend  = "weekend"
	NotificationTypeSystem    = "system"
	NotificationTypeCalendar = "calendar"
)

// Notification 统一通知
type Notification struct {
	ID        int64      `json:"id"`
	MemberID  int64      `json:"member_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	DataJSON  string     `json:"data_json,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	PushedAt *time.Time `json:"pushed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ToView 脱敏（不暴露内部字段）
func (n *Notification) ToView() *Notification {
	return &Notification{
		ID: n.ID, MemberID: n.MemberID, Type: n.Type, Title: n.Title,
		Body: n.Body, DataJSON: n.DataJSON, ReadAt: n.ReadAt,
		PushedAt: n.PushedAt, CreatedAt: n.CreatedAt,
	}
}
