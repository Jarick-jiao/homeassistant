package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// generateToken 生成随机 API Token（32 字节 = 64 字符 hex）
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "hm_" + hex.EncodeToString(b)
}

// ListAPITokensHandler 列出所有 API Token
func ListAPITokensHandler(c *gin.Context) {
	dbVal, _ := c.Get("db")
	db, ok := dbVal.(*store.DB)
	if !ok {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	tokens, err := db.ListAPITokens(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "查询失败")
		return
	}
	response.Success(c, tokens)
}

// CreateAPITokenHandler 创建 API Token（管理员）
func CreateAPITokenHandler(c *gin.Context) {
	dbVal, _ := c.Get("db")
	db, ok := dbVal.(*store.DB)
	if !ok {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	var req model.APITokenCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: name 和 scopes 必填")
		return
	}
	if len(req.Scopes) == 0 {
		response.BadRequest(c, "scopes 不能为空")
		return
	}

	// 校验 scope 合法性
	validScopes := map[string]bool{
		"news:write": true, "calendar:write": true,
		"weekend:write": true, "anniversary:write": true,
		"*": true,
	}
	for _, s := range req.Scopes {
		if !validScopes[s] {
			response.BadRequest(c, "无效的 scope: "+s)
			return
		}
	}

	tokenPlain := generateToken()
	var expiresAt *interface{}
	_ = expiresAt
	// 直接调用 store
	id, err := db.CreateAPIToken(c.Request.Context(), req.Name, tokenPlain, req.Scopes, nil)
	if err != nil {
		response.InternalServerError(c, "创建失败")
		return
	}

	response.Success(c, gin.H{
		"id":     id,
		"name":   req.Name,
		"scopes": req.Scopes,
		"token":  tokenPlain, // 仅此一次返回明文
		"notice": "请妥善保存此 Token，后续无法再次查看",
	})
}

// DeleteAPITokenHandler 撤销 API Token（管理员）
func DeleteAPITokenHandler(c *gin.Context) {
	dbVal, _ := c.Get("db")
	db, ok := dbVal.(*store.DB)
	if !ok {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := db.DeleteAPIToken(c.Request.Context(), id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, nil)
}
