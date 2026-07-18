package messageboard

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

// CreateMessageHandler 发送留言
func CreateMessageHandler(c *gin.Context) {
	var req struct {
		ToMemberID *int64 `json:"to_member_id"`
		Content    string `json:"content" binding:"required"`
		ParentID   *int64 `json:"parent_id"`
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
	fromMemberID := getCurrentMemberID(c)
	if fromMemberID == 0 {
		response.Unauthorized(c, "未识别到当前成员")
		return
	}
	m := &model.MessageBoard{
		FromMemberID: fromMemberID,
		ToMemberID:   req.ToMemberID,
		Content:      req.Content,
		ParentID:     req.ParentID,
	}
	id, err := db.CreateMessage(c.Request.Context(), m)
	if err != nil {
		response.InternalServerError(c, "发送失败: "+err.Error())
		return
	}
	// 如果 @某人，自动产生通知
	if req.ToMemberID != nil {
		fromName := db.GetMemberNameByID(c.Request.Context(), fromMemberID)
		notif := &model.Notification{
			MemberID: *req.ToMemberID,
			Type:     model.NotificationTypeMessage,
			Title:    "来自 " + fromName,
			Body:     req.Content,
			DataJSON: strconv.FormatInt(id, 10),
		}
		db.CreateNotification(c.Request.Context(), notif)
	}
	response.Success(c, gin.H{"id": id, "message": "留言已发送"})
}

// ListMessagesHandler 列出留言
func ListMessagesHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	memberID := getCurrentMemberID(c)
	msgs, err := db.ListMessages(c.Request.Context(), memberID)
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}
	// 补齐姓名
	views := make([]model.MessageBoard, 0, len(msgs))
	for _, m := range msgs {
		fromName := db.GetMemberNameByID(c.Request.Context(), m.FromMemberID)
		toName := ""
		if m.ToMemberID != nil {
			toName = db.GetMemberNameByID(c.Request.Context(), *m.ToMemberID)
		}
		view := *m.ToView(fromName, toName)
		views = append(views, view)
	}
	response.Success(c, views)
}

// GetMessageHandler 获取单条留言
func GetMessageHandler(c *gin.Context) {
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
	m, err := db.GetMessage(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "留言不存在")
		return
	}
	fromName := db.GetMemberNameByID(c.Request.Context(), m.FromMemberID)
	toName := ""
	if m.ToMemberID != nil {
		toName = db.GetMemberNameByID(c.Request.Context(), *m.ToMemberID)
	}
	response.Success(c, m.ToView(fromName, toName))
}

// MarkMessageReadHandler 标记已读
func MarkMessageReadHandler(c *gin.Context) {
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
	if err := db.MarkMessageRead(c.Request.Context(), id); err != nil {
		response.InternalServerError(c, "标记失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "read_at": time.Now()})
}

// DeleteMessageHandler 删除留言（admin 或作者，admin 由中间件保证）
func DeleteMessageHandler(c *gin.Context) {
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
	msg, err := db.GetMessage(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "留言不存在")
		return
	}
	currentMember := getCurrentMemberID(c)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(model.Role)
	if msg.FromMemberID != currentMember && role != model.RoleAdmin {
		response.Forbidden(c, "只能删除自己的留言")
		return
	}
	if err := db.DeleteMessage(c.Request.Context(), id); err != nil {
		response.InternalServerError(c, "删除失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true, "id": id})
}

// PinMessageHandler 置顶/取消置顶（admin only）
func PinMessageHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "ID 格式错误")
		return
	}
	var req struct {
		Pinned bool `json:"pinned"`
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
	if err := db.PinMessage(c.Request.Context(), id, req.Pinned); err != nil {
		response.InternalServerError(c, "操作失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "pinned": req.Pinned})
}
