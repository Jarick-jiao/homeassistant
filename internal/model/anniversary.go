package model

import "time"

// Anniversary 纪念日
type Anniversary struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`        // 如 "宝宝生日"
	Date        string    `json:"date"`         // YYYY-MM-DD
	Type        string    `json:"type"`         // birthday/wedding/memorial/custom
	MemberID    int64     `json:"member_id,omitempty"`
	Description string    `json:"description,omitempty"`
	IsLunar     bool      `json:"is_lunar"`     // 农历
	NotifyDays  int       `json:"notify_days"`  // 提前几天提醒
	CreatedBy   int64     `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// AnniversaryView 纪念日视图（含距今天数）
type AnniversaryView struct {
	Anniversary
	DaysUntil  int    `json:"days_until"`  // 距今天数（负值=已过）
	NextDate   string `json:"next_date"`   // 下次纪念日日期 YYYY-MM-DD
}

// AnniversaryCreateRequest 创建纪念日请求
type AnniversaryCreateRequest struct {
	Title       string `json:"title" binding:"required"`
	Date        string `json:"date" binding:"required"` // YYYY-MM-DD
	Type        string `json:"type"`
	MemberID    int64  `json:"member_id,omitempty"`
	Description string `json:"description,omitempty"`
	IsLunar     bool   `json:"is_lunar"`
	NotifyDays  int    `json:"notify_days,omitempty"`
}
