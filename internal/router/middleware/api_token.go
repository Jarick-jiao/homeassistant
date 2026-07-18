package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// APITokenAuth API Token 认证中间件
// requiredScope: 该接口所需的权限范围（如 "news:write"、"calendar:write"）
func APITokenAuth(requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取 token（支持 Authorization: Bearer <token> 或 X-API-Token: <token>）
		token := c.GetHeader("X-API-Token")
		if token == "" {
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if token == "" {
			response.Unauthorized(c, "缺少 API Token")
			c.Abort()
			return
		}

		// 2. 查询 token
		dbVal, exists := c.Get("db")
		if !exists || dbVal == nil {
			response.InternalServerError(c, "数据库不可用")
			c.Abort()
			return
		}
		db, ok := dbVal.(*store.DB)
		if !ok {
			response.InternalServerError(c, "数据库类型错误")
			c.Abort()
			return
		}

		hash := store.HashToken(token)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		apiToken, err := db.GetAPITokenByHash(ctx, hash)
		if err != nil || apiToken == nil {
			response.Unauthorized(c, "无效的 API Token")
			c.Abort()
			return
		}

		// 3. 校验过期时间
		if apiToken.ExpiresAt != nil && time.Now().After(*apiToken.ExpiresAt) {
			response.Unauthorized(c, "API Token 已过期")
			c.Abort()
			return
		}

		// 4. 校验 scope
		hasScope := false
		for _, s := range apiToken.Scopes {
			if s == requiredScope || s == "*" {
				hasScope = true
				break
			}
		}
		if !hasScope {
			response.Forbidden(c, "Token 无此操作权限（需要: "+requiredScope+"）")
			c.Abort()
			return
		}

		// 5. 更新最后使用时间（异步，不阻塞请求）
		go func(tokenID int64) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = db.UpdateAPITokenLastUsed(bgCtx, tokenID)
		}(apiToken.ID)

		// 6. 注入 token 信息
		c.Set("api_token_id", apiToken.ID)
		c.Set("api_token_name", apiToken.Name)
		c.Next()
	}
}
