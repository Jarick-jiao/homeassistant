package calendar

import (
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

// ListCalendarEventsHandler 列出日历事件
func ListCalendarEventsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	events, err := db.GetEvents(c.Request.Context(), 0)
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}
	response.Success(c, events)
}

// CreateCalendarEventHandler 创建日历事件
func CreateCalendarEventHandler(c *gin.Context) {
	var req struct {
		MemberID    int64  `json:"member_id"`
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Date        string `json:"date"`
		Time        string `json:"time"`
		Location    string `json:"location"`
		EventType   string `json:"event_type"`
		Type        string `json:"type"`
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
	// 简单解析日期
	var startTime, endTime time.Time
	if req.Date != "" && req.Time != "" {
		startTime, _ = time.Parse("2006-01-02 15:04", req.Date+" "+req.Time)
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}
	endTime = startTime.Add(2 * time.Hour)

	e := &model.CalendarEvent{
		MemberID: req.MemberID, Title: req.Title, Description: req.Description,
		StartTime: startTime, EndTime: endTime, Location: req.Location,
		EventType: req.EventType, Date: req.Date, Time: req.Time, Type: req.Type,
	}
	id, err := db.CreateEvent(c.Request.Context(), e)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "事件创建成功"})
}