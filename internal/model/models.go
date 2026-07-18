package model

import (
	"encoding/xml"
	"time"
)

// Role 角色类型
type Role string

// 角色常量定义
const (
	RoleAdmin Role = "admin"
	RoleAdult Role = "adult"
	RoleChild Role = "child"
	RoleElder Role = "elder"
	RoleGuest Role = "guest"
)

// Claims JWT claims 结构
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
	FamilyID int64  `json:"family_id"`
	IsAdmin  bool   `json:"is_admin"` // v3.6.0 系统管理员标记（admin 账号或被提升的成员）
}

// User 用户表
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Password     string    `json:"-"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Name         string    `json:"name,omitempty"`
	FamilyID     int64     `json:"family_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Member 家庭成员（handler 视图）
type Member struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Role             Role   `json:"role"`
	Age              int    `json:"age"`
	FamilyID         int64  `json:"family_id,omitempty"`
	HealthFocus      string `json:"health_focus,omitempty"`
	DataSourcePlugin string `json:"data_source_plugin,omitempty"`
	AvatarURL        string `json:"avatar_url,omitempty"`
	Bio              string `json:"bio,omitempty"`
	Password         string `json:"password,omitempty"` // 创建成员时设置登录密码
	IsAdmin          bool   `json:"is_admin"`            // v3.6.0 系统管理员标记
}

// FamilyMember 家庭成员表
type FamilyMember struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"user_id"`
	Name             string    `json:"name"`
	Role             string    `json:"role"`
	Age              int       `json:"age"`
	PreferencesJSON  string    `json:"preferences_json"`
	HealthFocus      string    `json:"health_focus"`
	DataSourcePlugin string    `json:"data_source_plugin"`
	AvatarURL        string    `json:"avatar_url"`
	Bio              string    `json:"bio"`
	IsAdmin          bool      `json:"is_admin"` // v3.6.0 系统管理员标记（叠加在家庭角色上）
	CreatedAt        time.Time `json:"created_at"`
}

// HealthRecord 健康记录表
type HealthRecord struct {
	ID         int64     `json:"id"`
	MemberID   int64     `json:"member_id"`
	Type       string    `json:"type,omitempty"`
	RecordType string    `json:"record_type,omitempty"`
	Value      interface{} `json:"value,omitempty"`
	Unit       string    `json:"unit,omitempty"`
	RecordAt   string    `json:"record_at,omitempty"`
	Note       string    `json:"note,omitempty"`
	RecordedAt string    `json:"recorded_at,omitempty"`
	Source     string    `json:"source,omitempty"`
}

// CalendarEvent 日历事件表
type CalendarEvent struct {
	ID              int64      `json:"id"`
	MemberID        int64      `json:"member_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	StartTime       time.Time  `json:"start_time,omitempty"`
	EndTime         time.Time  `json:"end_time,omitempty"`
	Date            string     `json:"date,omitempty"`
	Time            string     `json:"time,omitempty"`
	Location        string     `json:"location"`
	EventType       string     `json:"event_type,omitempty"`
	Type            string     `json:"type,omitempty"`
	IsImportant     bool       `json:"is_important"`
	RecurrenceRule  string     `json:"recurrence_rule,omitempty"`
	ReminderMinutes int        `json:"reminder_minutes"`
	LastRemindedAt  *time.Time `json:"last_reminded_at,omitempty"`
	CreatedBy       int64      `json:"created_by"`
	Color           string     `json:"color,omitempty"`
}

// RecurrenceRule 周期规则（简单子集，不实现完整 RFC 5545 RRULE）
type RecurrenceRule struct {
	Freq      string `json:"freq"`                // daily/weekly/monthly/yearly
	Interval  int    `json:"interval"`             // 间隔，默认 1
	Until     string `json:"until,omitempty"`     // YYYY-MM-DD 截止日期
	Count     int    `json:"count,omitempty"`     // 重复次数（与 until 二选一）
	ByWeekday []int  `json:"byweekday,omitempty"` // 0=周日..6=周六，仅 weekly 用
}

// TripPlan 出行计划表
type TripPlan struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Destination string  `json:"destination"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	Status      string  `json:"status"`
	PlanJSON    string  `json:"plan_json"`
	CreatedBy   int64   `json:"created_by"`
	MembersJSON string  `json:"members_json,omitempty"`
}

// ChatMessage 聊天记录表
type ChatMessage struct {
	ID        int64  `json:"id"`
	MemberID  int64  `json:"member_id"`
	Content   string `json:"content"`
	Role      string `json:"role"`
	Timestamp int64  `json:"timestamp"`
	SessionID string `json:"session_id"`
}

// Reminder 提醒事项表
type Reminder struct {
	ID       int64     `json:"id"`
	MemberID int64     `json:"member_id"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	RemindAt time.Time `json:"remind_at"`
	Status   string    `json:"status"`
	Channel  string    `json:"channel"`
}

// DeviceData 设备数据表
type DeviceData struct {
	ID         int64     `json:"id"`
	MemberID   int64     `json:"member_id"`
	DeviceType string    `json:"device_type"`
	DataJSON   string    `json:"data_json"`
	ReceivedAt time.Time `json:"received_at"`
}

// FeedbackRecord 反馈记录表
type FeedbackRecord struct {
	ID        int64     `json:"id"`
	MemberID  int64     `json:"member_id"`
	ItemName  string    `json:"item_name"`
	ItemType  string    `json:"item_type"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// Feedback 反馈（handler 视图）
type Feedback struct {
	ID        int64  `json:"id"`
	MemberID  int64  `json:"member_id"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Rating    int    `json:"rating"`
	CreatedAt string `json:"created_at"`
}

// IoTDevice IoT 设备（handler 视图）
type IoTDevice struct {
	ID       int64                  `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	EntityID string                 `json:"entity_id"`
	State    string                 `json:"state"`
	Attrs    map[string]interface{} `json:"attrs"`
}

// WeChatMessage 微信消息
type WeChatMessage struct {
	ToUserName   string `xml:"ToUserName" json:"to_user_name"`
	FromUserName string `xml:"FromUserName" json:"from_user_name"`
	CreateTime   int64  `xml:"CreateTime" json:"create_time"`
	MsgType      string `xml:"MsgType" json:"msg_type"`
	Content      string `xml:"Content" json:"content"`
	MsgId        int64  `xml:"MsgId" json:"msg_id"`
}

// WeChatResponse 微信回复消息
type WeChatResponse struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
}

// DataSourceConfig 成员数据源配置
type DataSourceConfig struct {
	ID         int64  `json:"id"`
	MemberID   int64  `json:"member_id"`
	MemberName string `json:"member_name"`
	SourceType string `json:"source_type"`
	APIKey     string `json:"api_key,omitempty"`
	APISecret  string `json:"api_secret,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	IsActive   bool   `json:"is_active"`
	LastSyncAt string `json:"last_sync_at,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// DataSourceConfigView 数据源配置视图（API 返回用，API Key 脱敏）
type DataSourceConfigView struct {
	ID          int64  `json:"id"`
	MemberID    int64  `json:"member_id"`
	MemberName  string `json:"member_name"`
	SourceType  string `json:"source_type"`
	APIKeyHint  string `json:"api_key_hint"`
	UserID      string `json:"user_id,omitempty"`
	IsActive    bool   `json:"is_active"`
	LastSyncAt  string `json:"last_sync_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// ToView 将 DataSourceConfig 转为脱敏的 DataSourceConfigView
func (c *DataSourceConfig) ToView() *DataSourceConfigView {
	hint := ""
	if len(c.APIKey) >= 4 {
		hint = c.APIKey[:4] + "****"
	} else if c.APIKey != "" {
		hint = "****"
	}
	return &DataSourceConfigView{
		ID:          c.ID,
		MemberID:    c.MemberID,
		MemberName:  c.MemberName,
		SourceType:  c.SourceType,
		APIKeyHint:  hint,
		UserID:      c.UserID,
		IsActive:    c.IsActive,
		LastSyncAt:  c.LastSyncAt,
		CreatedAt:   c.CreatedAt,
	}
}

// HealthDataCache 健康数据缓存
type HealthDataCache struct {
	ID               int64   `json:"id"`
	MemberID         int64   `json:"member_id"`
	Date             string  `json:"date"`
	Steps            int     `json:"steps"`
	HeartRate        int     `json:"heart_rate"`
	SleepHours       float64 `json:"sleep_hours"`
	SleepScore       int     `json:"sleep_score"`
	BloodPressureSys int     `json:"blood_pressure_sys"`
	BloodPressureDia int     `json:"blood_pressure_dia"`
	Weight           float64 `json:"weight"`
	Height           float64 `json:"height"`
	Stress           int     `json:"stress"`
	SpO2             int     `json:"spo2"`
	BodyBattery      int     `json:"body_battery"`
	Calories         int     `json:"calories"`
	Source           string  `json:"source"`
	SyncedAt         string  `json:"synced_at"`
}

// ============ 新增数据库模型 ============

// ChorseTaskDB 家务任务（数据库模型）
type ChorseTaskDB struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
	Difficulty  string `json:"difficulty"`
	Points      int    `json:"points"`
	Duration    string `json:"duration"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// ChorseClaimDB 家务认领记录（数据库模型）
type ChorseClaimDB struct {
	ID           int64      `json:"id"`
	TaskID       int64      `json:"task_id"`
	TaskName     string     `json:"task_name"`
	TaskIcon     string     `json:"task_icon"`
	MemberID     int64      `json:"member_id"`
	MemberName   string     `json:"member_name"`
	ClaimedAt    time.Time  `json:"claimed_at"`
	Deadline     *time.Time `json:"deadline"`
	Status       string     `json:"status"` // pending/completed/confirmed
	Points       int        `json:"points"`
	VerifierID   int64      `json:"verifier_id"`
	VerifierName string     `json:"verifier_name"`
	ConfirmedBy  string     `json:"confirmed_by"`
	ConfirmedAt  *time.Time `json:"confirmed_at"`
}

// WeekendProposalDB 周末出行方案（数据库模型）
type WeekendProposalDB struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
	TagsJSON    string `json:"tags_json"`
	Duration    string `json:"duration"`
	Cost        string `json:"cost"`
	Difficulty  string `json:"difficulty"`
	SuitableFor string `json:"suitable_for"`
	WeatherReq  string `json:"weather_req"`
	Tips        string `json:"tips"`
	CreatedBy   int64  `json:"created_by,omitempty"`
}