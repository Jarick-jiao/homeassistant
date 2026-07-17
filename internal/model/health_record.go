package model

import "time"

// HealthRecordFile 健康档案文件
type HealthRecordFile struct {
	ID          int64     `json:"id"`
	MemberID   int64     `json:"member_id"`
	Title       string    `json:"title"`
	Category    string    `json:"category"`    // 病例/检查报告/处方/化验单/影像/其他
	RecordDate  string    `json:"record_date"`  // 病历/检查日期
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	FileType    string    `json:"file_type"`    // pdf/jpg/png
	FilePath    string    `json:"file_path"`    // 本地存储路径（相对 uploads/）
	ThumbPath   string    `json:"thumb_path"`
	Summary     string    `json:"summary"`      // AI 摘要
	Analysis    string    `json:"analysis"`     // AI 分析结果（JSON 格式）
	AnalyzedAt  time.Time `json:"analyzed_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// HealthRecordFileView 前端视图
type HealthRecordFileView struct {
	ID          int64  `json:"id"`
	MemberID    int64  `json:"member_id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	RecordDate  string `json:"record_date"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	FileType    string `json:"file_type"`
	FileURL     string `json:"file_url"`
	Summary     string `json:"summary"`
	Analysis    string `json:"analysis"`
	IsAnalyzed  bool   `json:"is_analyzed"`
}

// HealthAnalysisReport AI 健康分析报告
type HealthAnalysisReport struct {
	ID          int64     `json:"id"`
	MemberID    int64     `json:"member_id"`
	ReportDate  string    `json:"report_date"`
	PeriodStart string    `json:"period_start"`
	PeriodEnd   string    `json:"period_end"`
	Summary     string    `json:"summary"`
	Details     string    `json:"details"`     // Markdown 格式详细报告
	Metrics     string    `json:"metrics"`     // JSON 格式指标汇总
	Source      string    `json:"source"`      // manual/ai
	CreatedAt   time.Time `json:"created_at"`
}

// WeekendRecommendation 周末推荐方案（AI 生成 / 离线缓存）
type WeekendRecommendation struct {
	ID           int64     `json:"id"`
	GeneratedFor string    `json:"generated_for"` // all / member:{id}
	WeekendDate  string    `json:"weekend_date"`
	WeatherData  string    `json:"weather_data"` // JSON 天气快照
	Proposals    string    `json:"proposals"`   // JSON 推荐方案列表
	Source       string    `json:"source"`      // ai / offline
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}