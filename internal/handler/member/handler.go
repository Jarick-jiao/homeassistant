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
