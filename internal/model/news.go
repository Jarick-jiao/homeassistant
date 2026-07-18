package model

import "time"

// News 时事新闻
type News struct {
	ID          int64     `json:"id"`
	Category    string    `json:"category"`     // tech/world/local/sports/finance/health
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Content     string    `json:"content,omitempty"`
	Source      string    `json:"source"`        // rss/manual/api
	SourceURL   string    `json:"source_url,omitempty"`
	ImageURL    string    `json:"image_url,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	IsHot       bool      `json:"is_hot"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewsCreateRequest 外部写入新闻请求
type NewsCreateRequest struct {
	Category    string `json:"category" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Summary     string `json:"summary"`
	Content     string `json:"content,omitempty"`
	Source      string `json:"source"`
	SourceURL   string `json:"source_url,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"` // RFC3339 或 YYYY-MM-DD HH:MM:SS
	IsHot       bool   `json:"is_hot"`
}
