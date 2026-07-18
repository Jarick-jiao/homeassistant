package notification

import (
	"strconv"

	"github.com/gin-gonic/gin"
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

// ListNotificationsHandler 列出当前成员通知
func ListNotificationsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	memberID := getCurrentMemberID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	list, err := db.ListNotifications(c.Request.Context(), memberID, limit, offset)
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}
	views := make([]interface{}, 0, len(list))
	for _, n := range list {
		views = append(views, n.ToView())
	}
	response.Success(c, views)
}

// UnreadCountHandler 未读数量
func UnreadCountHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, gin.H{"count": 0})
		return
	}
	memberID := getCurrentMemberID(c)
	count, _ := db.UnreadNotificationCount(c.Request.Context(), memberID)
	response.Success(c, gin.H{"count": count})
}

// MarkReadHandler 标记已读
func MarkReadHandler(c *gin.Context) {
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
	if err := db.MarkNotificationRead(c.Request.Context(), id); err != nil {
		response.InternalServerError(c, "标记失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "read": true})
}

// MarkAllReadHandler 全部已读
func MarkAllReadHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	memberID := getCurrentMemberID(c)
	if err := db.MarkAllNotificationsRead(c.Request.Context(), memberID); err != nil {
		response.InternalServerError(c, "操作失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"marked_all": true})
}
