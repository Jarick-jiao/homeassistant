package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/jwtutil"
	"github.com/homemate/server/internal/pkg/response"
)

// JWTSecretHolder 用于从外部注入 JWT Secret 的接口
type JWTSecretHolder interface {
	GetJWTSecret() string
}

// AuthMiddleware JWT 认证中间件
func AuthMiddleware(secretHolder JWTSecretHolder) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "缺少 Authorization 请求头")
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			response.Unauthorized(c, "Authorization 格式错误，应为 Bearer {token}")
			c.Abort()
			return
		}
		secret := secretHolder.GetJWTSecret()
		claims, err := jwtutil.ParseToken(parts[1], secret)
		if err != nil {
			response.Unauthorized(c, "无效的 Token: "+err.Error())
			c.Abort()
			return
		}
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("familyID", claims.FamilyID)
		c.Set("claims", claims)
		c.Next()
	}
}