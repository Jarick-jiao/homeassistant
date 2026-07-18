package reminder

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

func getDB(c *gin.Context) *store.DB {
	dbVal, _ := c.Get("db")
	if dbVal == nil {
		return nil
	}
	return dbVal.(*store.DB)
}

func getCurrentMemberID(c *gin.Context) int64 {
	userIDVal, _ := c.Get("userID")
	userID, _ := userIDVal.(int64)
	db := getDB(c)
	if db == nil || userID == 0 {
		return 0
	}
	memberID, _ := db.GetMemberIDByUserID(c.Request.Context(), userID)
	return memberID
}

// ListRemindersHandler 列出当前成员提醒
func ListRemindersHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	memberID := getCurrentMemberID(c)
	list, err := db.ListRemindersByMember(c.Request.Context(), memberID, "")
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}
	response.Success(c, list)
}

// CreateReminderHandler 创建提醒
func CreateReminderHandler(c *gin.Context) {
	var req struct {
		Title    string `json:"title" binding:"required"`
		Content  string `json:"content"`
		RemindAt string `json:"remind_at" binding:"required"`
		Channel  string `json:"channel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	memberID := getCurrentMemberID(c)
	remindAt, err := time.Parse(time.RFC3339, req.RemindAt)
	if err != nil {
		remindAt, err = time.Parse("2006-01-02 15:04", req.RemindAt)
		if err != nil {
			response.BadRequest(c, "时间格式错误，应为 RFC3339 或 2006-01-02 15:04")
			return
		}
	}
	if req.Channel == "" {
		req.Channel = "app"
	}
	r := &model.Reminder{
		MemberID: memberID,
		Title:    req.Title,
		Content:  req.Content,
		RemindAt: remindAt,
		Status:   "pending",
		Channel:  req.Channel,
	}
	id, err := db.CreateReminder(c.Request.Context(), r)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "提醒已创建"})
}

// UpdateReminderHandler 更新提醒
func UpdateReminderHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "ID 格式错误")
		return
	}
	var req struct {
		Title    string `json:"title"`
		Content  string `json:"content"`
		RemindAt string `json:"remind_at"`
		Channel  string `json:"channel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	r := &model.Reminder{ID: id, Title: req.Title, Content: req.Content, Channel: req.Channel}
	if req.RemindAt != "" {
		remindAt, err := time.Parse(time.RFC3339, req.RemindAt)
		if err != nil {
			remindAt, err = time.Parse("2006-01-02 15:04", req.RemindAt)
			if err != nil {
				response.BadRequest(c, "时间格式错误")
				return
			}
		}
		r.RemindAt = remindAt
	}
	if err := db.UpdateReminder(c.Request.Context(), r); err != nil {
		response.InternalServerError(c, "更新失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "提醒已更新"})
}

// DeleteReminderHandler 删除提醒（admin 或本人，admin 由中间件保证）
func DeleteReminderHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "ID 格式错误")
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	// 作者校验（admin 已由中间件保证）
	currentMember := getCurrentMemberID(c)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(model.Role)
	isAdminVal, _ := c.Get("isAdmin")
	isAdmin, _ := isAdminVal.(bool)
	list, _ := db.ListRemindersByMember(c.Request.Context(), currentMember, "")
	isOwner := false
	for _, r := range list {
		if r.ID == id {
			isOwner = true
			break
		}
	}
	if !isOwner && role != model.RoleAdmin && !isAdmin {
		response.Forbidden(c, "只能删除自己的提醒")
		return
	}
	if err := db.DeleteReminder(c.Request.Context(), id); err != nil {
		response.InternalServerError(c, "删除失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true, "id": id})
}
