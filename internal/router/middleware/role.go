package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

// v4.0：基于路径白名单的 RoleMiddleware 已删除（从未挂载且路径归一化存在缺陷）。
// 授权模型统一为（范式 §2.2）：
//   1. AuthMiddleware 认证 JWT
//   2. 管理员路由显式挂载 RequireAdmin()
//   3. 家庭业务接口在 handler 内做「JWT → 家庭成员」身份绑定与资源属主校验
//     （见 internal/pkg/memberctx）
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// v3.6.0: 被提升为管理员的成员（is_admin=true）拥有全部权限，等价于 role='admin'
		if IsAdmin(c) {
			c.Next()
			return
		}
		roleVal, exists := c.Get("role")
		if !exists {
			response.Forbidden(c, "未获取到用户角色信息")
			c.Abort()
			return
		}
		role, ok := roleVal.(model.Role)
		if !ok {
			response.Forbidden(c, "用户角色类型错误")
			c.Abort()
			return
		}
		for _, r := range roles {
			if string(role) == r {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "权限不足，需要 "+strings.Join(roles, "/")+" 角色")
		c.Abort()
	}
}

// IsAdmin 判断当前用户是否为系统管理员
// v3.6.0: admin 账号（role='admin'）或被提升的成员（is_admin=true）均视为系统管理员
func IsAdmin(c *gin.Context) bool {
	if isAdminVal, exists := c.Get("isAdmin"); exists {
		if isAdmin, ok := isAdminVal.(bool); ok && isAdmin {
			return true
		}
	}
	if roleVal, exists := c.Get("role"); exists {
		if role, ok := roleVal.(model.Role); ok && role == model.RoleAdmin {
			return true
		}
	}
	return false
}

// RequireAdmin 要求系统管理员权限（v3.6.0: admin 账号或被提升的成员）
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsAdmin(c) {
			c.Next()
			return
		}
		response.Forbidden(c, "权限不足，需要系统管理员权限")
		c.Abort()
	}
}