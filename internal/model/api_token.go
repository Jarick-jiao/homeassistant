package model

import "time"

// APIToken API 令牌（用于外部程序接入）
type APIToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`        // 令牌名称（如"RSS同步器"）
	TokenHash  string     `json:"-"`           // SHA256 哈希，不返回
	ScopesJSON string     `json:"-"`           // JSON 数组字符串
	Scopes     []string   `json:"scopes"`      // 权限范围
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// APITokenView 脱敏视图
type APITokenView struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// ToView 转为视图
func (t *APIToken) ToView() *APITokenView {
	return &APITokenView{
		ID: t.ID, Name: t.Name, Scopes: t.Scopes,
		IsActive: t.IsActive, CreatedAt: t.CreatedAt,
		LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt,
	}
}

// APITokenCreateRequest 创建令牌请求
type APITokenCreateRequest struct {
	Name      string   `json:"name" binding:"required"`
	Scopes    []string `json:"scopes" binding:"required"`
	ExpiresIn int      `json:"expires_in,omitempty"` // 秒，0=永不过期
}
