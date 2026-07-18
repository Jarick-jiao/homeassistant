package anniversary

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

func getDB(c *gin.Context) *store.DB {
	if dbVal, exists := c.Get("db"); exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			return db
		}
	}
	return nil
}

// ListAnniversariesHandler 列出全部纪念日（公开）
func ListAnniversariesHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	list, err := db.ListAnniversaries(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "查询失败")
		return
	}
	response.Success(c, list)
}

// GetUpcomingAnniversariesHandler 获取未来 N 天的纪念日（公开，默认 30 天）
func GetUpcomingAnniversariesHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days <= 0 || days > 365 {
		days = 30
	}
	list, err := db.GetUpcomingAnniversaries(c.Request.Context(), days)
	if err != nil {
		response.InternalServerError(c, "查询失败")
		return
	}
	response.Success(c, list)
}

// CreateAnniversaryHandler 创建纪念日（API Token: anniversary:write 或 JWT）
func CreateAnniversaryHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	var req model.AnniversaryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: title 和 date 必填")
		return
	}
	// created_by: 优先取 JWT userID，否则取 API Token（0）
	var createdBy int64
	if uid, exists := c.Get("userID"); exists {
		if id, ok := uid.(int64); ok {
			createdBy = id
		}
	}
	a := &model.Anniversary{
		Title:       req.Title,
		Date:        req.Date,
		Type:        req.Type,
		MemberID:    req.MemberID,
		Description: req.Description,
		IsLunar:     req.IsLunar,
		NotifyDays:  req.NotifyDays,
		CreatedBy:   createdBy,
	}
	id, err := db.CreateAnniversary(c.Request.Context(), a)
	if err != nil {
		response.InternalServerError(c, "创建失败")
		return
	}
	response.Success(c, gin.H{"id": id})
}

// UpdateAnniversaryHandler 更新纪念日（管理员或 API Token）
func UpdateAnniversaryHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的 ID")
		return
	}
	var req model.AnniversaryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	a := &model.Anniversary{
		Title: req.Title, Date: req.Date, Type: req.Type,
		MemberID: req.MemberID, Description: req.Description,
		IsLunar: req.IsLunar, NotifyDays: req.NotifyDays,
	}
	if err := db.UpdateAnniversary(c.Request.Context(), id, a); err != nil {
		response.NotFound(c, "纪念日不存在")
		return
	}
	response.Success(c, nil)
}

// DeleteAnniversaryHandler 删除纪念日（管理员或 API Token）
func DeleteAnniversaryHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := db.DeleteAnniversary(c.Request.Context(), id); err != nil {
		response.NotFound(c, "纪念日不存在")
		return
	}
	response.Success(c, nil)
}
