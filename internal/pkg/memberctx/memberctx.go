// Package memberctx 从 JWT 上下文解析当前家庭成员。
// 范式 §2.2：业务身份（认领人/投票人/兑换人/验收人）一律由后端从 JWT 解析，
// 禁止信任请求体中的 member_id/member_name/confirmer 等字段。
package memberctx

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/store"
)

var (
	// ErrDBMissing 上下文未注入数据库
	ErrDBMissing = errors.New("数据库未初始化")
	// ErrNotBound 当前登录账号未绑定家庭成员档案
	ErrNotBound = errors.New("当前账号未绑定家庭成员，请联系管理员创建成员档案")
)

// GetDB 从 gin.Context 取出 store.DB
func GetDB(c *gin.Context) *store.DB {
	v, ok := c.Get("db")
	if !ok {
		return nil
	}
	db, _ := v.(*store.DB)
	return db
}

// CurrentMember 解析当前登录用户对应的家庭成员。
// 未登录、无 userID 或未绑定 family_members 时返回 ErrNotBound。
func CurrentMember(c *gin.Context) (*model.FamilyMember, error) {
	db := GetDB(c)
	if db == nil {
		return nil, ErrDBMissing
	}
	raw, exists := c.Get("userID")
	if !exists {
		return nil, ErrNotBound
	}
	userID, _ := raw.(int64)
	if userID == 0 {
		return nil, ErrNotBound
	}
	m, err := db.GetMemberByUserID(c.Request.Context(), userID)
	if err != nil || m == nil {
		return nil, ErrNotBound
	}
	return m, nil
}

// IsAdmin 当前 JWT 是否系统管理员
func IsAdmin(c *gin.Context) bool {
	v, exists := c.Get("isAdmin")
	if !exists {
		return false
	}
	b, _ := v.(bool)
	return b
}

// Username 当前登录用户名（用于操作人记录）
func Username(c *gin.Context) string {
	v, _ := c.Get("username")
	s, _ := v.(string)
	return s
}
