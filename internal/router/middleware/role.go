package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

// rolePermissions 定义各角色可访问的精确路由路径
// 使用精确匹配（不再使用 HasPrefix，防止 /api/chat 放行 /api/chat-admin 等路径）
// 支持 "GET:/api/health/overview" 格式的方法+路径匹配，或纯路径匹配所有方法
var rolePermissions = map[model.Role][]string{
	model.RoleAdmin: {"*"},
	model.RoleAdult: {
		"/api/health/overview",
		"/api/health/records",
		"/api/health/real-data",
		"/api/health/data-source/configs",
		"/api/health/data-source/config",
		"/api/health/sync",
		"/api/calendar/events",
		"/api/calendar/upcoming",
		"/api/chorse/dashboard",
		"/api/chorse/claims",
		"/api/chorse/complete",
		"/api/chorse/confirm",
		"/api/points/dashboard",
		"/api/weekend/dashboard",
		"/api/weekend/proposals",
		"/api/records",
		"/api/scheduler/status",
		"/api/messages",
		"/api/notifications",
		"/api/reminders",
	},
	model.RoleChild: {
		"/api/health/overview",
		"/api/chorse/dashboard",
		"/api/points/dashboard",
		"/api/calendar/events",
		"/api/calendar/upcoming",
		"/api/messages",
		"/api/notifications",
		"/api/weekend/dashboard",
		"/api/weekend/proposals",
		"/api/weekend/vote",
	},
	model.RoleElder: {
		"/api/health/overview",
		"/api/health/records",
		"/api/health/real-data",
		"/api/health/data-source/configs",
		"/api/calendar/events",
		"/api/calendar/upcoming",
		"/api/records",
		"/api/records/reports",
		"/api/messages",
		"/api/notifications",
		"/api/reminders",
	},
	model.RoleGuest: {
		"/api/health/overview",
		"/api/notifications",
		"/api/calendar/upcoming",
	},
}

// normalizePath 去除路径参数部分，得到路由模板用于权限比较
// 例如 /api/records/123 → /api/records, /api/chorse/claims/45/complete → /api/chorse/complete
func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	// 保留前3段（/api/xxx），如果第4段是数字（ID）则跳过
	// /api/records/:id/download → /api/records/download
	// /api/chorse/claims/:id/complete → /api/chorse/complete
	result := make([]string, 0, len(parts))
	for i, p := range parts {
		if i >= 3 && i%2 == 1 && len(p) > 0 {
			// 检查是否是路径参数位置（奇数索引 >= 3 通常是 :id）
			continue
		}
		result = append(result, p)
	}
	return strings.Join(result, "/")
}

// matchPermission 检查请求路径是否匹配权限规则
// 规则格式：纯路径匹配所有方法
func matchPermission(rule, path string) bool {
	if rule == "*" {
		return true
	}
	return normalizePath(path) == rule
}
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
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

// RequireAdmin 要求管理员角色（用于所有 DELETE 等高危操作）
func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

// RoleMiddleware 基于角色的访问控制中间件
func RoleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		path := c.Request.URL.Path

		if role == model.RoleAdmin {
			c.Next()
			return
		}

		allowedPaths, exists := rolePermissions[role]
		if !exists {
			response.Forbidden(c, "未知角色，无权限访问")
			c.Abort()
			return
		}
		for _, rule := range allowedPaths {
			if matchPermission(rule, path) {
				c.Next()
				return
			}
		}
		response.Forbidden(c, "当前角色无权限访问该资源")
		c.Abort()
	}
}