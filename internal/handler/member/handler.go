package member

import (
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// ListMembersHandler 列出家庭成员
// 从数据库查询，无数据时返回空数组
func ListMembersHandler(c *gin.Context) {
	familyID, _ := c.Get("familyID")
	_ = familyID

	// 尝试从数据库查询
	dbVal, exists := c.Get("db")
	if exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			familyMembers, err := db.GetMembers(c.Request.Context())
			if err == nil {
				members := make([]model.Member, 0, len(familyMembers))
				for _, fm := range familyMembers {
					members = append(members, model.Member{
					ID:               fm.ID,
					Name:             fm.Name,
					Role:             model.Role(fm.Role),
					Age:              fm.Age,
					HealthFocus:      fm.HealthFocus,
					DataSourcePlugin: fm.DataSourcePlugin,
					AvatarURL:        fm.AvatarURL,
					Bio:              fm.Bio,
					IsAdmin:          fm.IsAdmin,
				})
				}
				response.Success(c, members)
				return
			}
		}
	}

	// 数据库不可用或无数据，返回空数组
	response.Success(c, []model.Member{})
}

// GetMemberDetailHandler 获取单个成员详情
func GetMemberDetailHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "成员ID格式错误")
		return
	}

	dbVal, exists := c.Get("db")
	if exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			fm, err := db.GetMemberByID(c.Request.Context(), id)
			if err == nil && fm != nil {
				response.Success(c, model.Member{
					ID:               fm.ID,
					Name:             fm.Name,
					Role:             model.Role(fm.Role),
					Age:              fm.Age,
					HealthFocus:      fm.HealthFocus,
					DataSourcePlugin: fm.DataSourcePlugin,
					AvatarURL:        fm.AvatarURL,
					Bio:              fm.Bio,
					IsAdmin:          fm.IsAdmin,
				})
				return
			}
		}
	}

	response.Error(c, 404, "成员不存在")
}

// CreateMemberHandler 创建家庭成员
func CreateMemberHandler(c *gin.Context) {
	var req model.Member
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 参数校验
	if req.Name == "" {
		response.BadRequest(c, "成员姓名不能为空")
		return
	}
	if req.Role == "" {
		req.Role = model.RoleGuest
	}
	// v3.6.0: admin 不再作为家庭角色，强制改为 adult
	if req.Role == model.RoleAdmin {
		req.Role = model.RoleAdult
	}
	if req.DataSourcePlugin == "" {
		req.DataSourcePlugin = "manual"
	}

	// 尝试写入数据库
	dbVal, exists := c.Get("db")
	if exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			fm := &model.FamilyMember{
				Name:             req.Name,
				Role:             string(req.Role),
				Age:              req.Age,
				HealthFocus:      req.HealthFocus,
				DataSourcePlugin: req.DataSourcePlugin,
				AvatarURL:        req.AvatarURL,
				Bio:              req.Bio,
				IsAdmin:          req.IsAdmin,
			}
			id, err := db.CreateMember(c.Request.Context(), fm)
			if err != nil {
				response.Error(c, 500, "创建成员失败: "+err.Error())
				return
			}
			req.ID = id

			// 如果提供了密码，自动创建登录账号
			if req.Password != "" {
				hash, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
				if hashErr == nil {
					user := &model.User{
						Username:     req.Name,
						PasswordHash: string(hash),
						Role:         req.Role,
						Name:         req.Name,
						FamilyID:     1,
					}
					if _, createUserErr := db.CreateUser(c.Request.Context(), user); createUserErr != nil {
						// 账号创建失败不影响成员创建，仅日志记录
						log.Printf("[WARN] 自动创建用户账号失败 %s: %v", req.Name, createUserErr)
					}
				}
			}

			response.Success(c, req)
			return
		}
	}

	response.Error(c, 500, "数据库不可用，无法创建成员")
}

// UpdateMemberHandler 更新家庭成员
func UpdateMemberHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "成员ID格式错误")
		return
	}

	var req model.Member
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// v3.6.0: admin 不再作为家庭角色，强制改为 adult
	if req.Role == model.RoleAdmin {
		req.Role = model.RoleAdult
	}

	req.ID = id // 确保返回正确的ID

	dbVal, exists := c.Get("db")
	if exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			fm := &model.FamilyMember{
				ID:               id,
				Name:             req.Name,
				Role:             string(req.Role),
				Age:              req.Age,
				HealthFocus:      req.HealthFocus,
				DataSourcePlugin: req.DataSourcePlugin,
				AvatarURL:        req.AvatarURL,
				Bio:              req.Bio,
				IsAdmin:          req.IsAdmin,
			}
			if err := db.UpdateMember(c.Request.Context(), fm); err == nil {
				response.Success(c, req)
				return
			}
		}
	}

	response.Success(c, req)
}

// DeleteMemberHandler 删除家庭成员
func DeleteMemberHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "成员ID格式错误")
		return
	}

	dbVal, exists := c.Get("db")
	if exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			// 校验成员是否存在
			if _, err := db.GetMemberByID(c.Request.Context(), id); err != nil {
				response.Error(c, 404, "成员不存在")
				return
			}
			// 执行删除
			if err := db.DeleteMember(c.Request.Context(), id); err != nil {
				response.Error(c, 500, "删除成员失败: "+err.Error())
				return
			}
			response.Success(c, gin.H{"deleted": true, "id": id})
			return
		}
	}

	response.Error(c, 500, "数据库不可用，无法删除成员")
}

// UpdateMemberRoleHandler 更新成员家庭角色
// v3.6.0: admin 不再作为家庭角色，仅允许 adult/child/elder/guest
// 管理员权限的委派/撤销请使用 PUT /api/members/:id/admin
func UpdateMemberRoleHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "成员ID格式错误")
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// v3.6.0: 校验角色合法性（admin 不再作为家庭角色）
	validRoles := map[string]bool{
		"adult": true, "child": true, "elder": true, "guest": true, "pet": true,
	}
	if !validRoles[req.Role] {
		response.BadRequest(c, "非法角色: "+req.Role+"（admin 不再作为家庭角色，请使用管理员开关）")
		return
	}

	// 禁止操作自己的角色
	currentUserID, _ := c.Get("userID")
	uid, _ := currentUserID.(int64)
	dbVal, _ := c.Get("db")
	db, ok := dbVal.(*store.DB)
	if !ok || db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	member, err := db.GetMemberByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "成员不存在")
		return
	}
	if member.UserID == uid {
		response.BadRequest(c, "不能修改自己的角色")
		return
	}
	if err := db.UpdateMemberRole(c.Request.Context(), id, req.Role); err != nil {
		response.InternalServerError(c, "更新角色失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "role": req.Role, "message": "角色已更新"})
}

// UpdateMemberAdminHandler v3.6.0 切换成员的系统管理员标记（is_admin）
// 用于委派/撤销管理员权限，叠加在家庭角色上，不替换 role
// 保护：禁止操作自己；撤销时检查是否是最后一个管理员
func UpdateMemberAdminHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "成员ID格式错误")
		return
	}

	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	dbVal, _ := c.Get("db")
	db, ok := dbVal.(*store.DB)
	if !ok || db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}

	member, err := db.GetMemberByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "成员不存在")
		return
	}

	// 禁止操作自己的管理员权限（防零管理员锁死）
	currentUserID, _ := c.Get("userID")
	uid, _ := currentUserID.(int64)
	if member.UserID == uid {
		response.BadRequest(c, "不能修改自己的管理员权限")
		return
	}

	// 撤销管理员时，检查是否是最后一个管理员
	if member.IsAdmin && !req.IsAdmin {
		count, err := db.CountAdmins(c.Request.Context())
		if err == nil && count <= 1 {
			response.BadRequest(c, "不能撤销最后一个管理员，请先委派其他管理员")
			return
		}
	}

	if err := db.UpdateMemberAdmin(c.Request.Context(), id, req.IsAdmin); err != nil {
		response.InternalServerError(c, "更新管理员标记失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "is_admin": req.IsAdmin, "message": "管理员权限已更新"})
}
