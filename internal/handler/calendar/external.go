package calendar

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/service/calendar"
)

// ExternalCreateEventHandler 外部写入日历事件（需 API Token: calendar:write）
// 逻辑与 CreateCalendarEventHandler 一致，但 created_by 标记为系统(0)
func ExternalCreateEventHandler(c *gin.Context) {
	var req struct {
		MemberID         int64  `json:"member_id"`
		Title            string `json:"title" binding:"required"`
		Description      string `json:"description"`
		StartTime        string `json:"start_time"`
		EndTime          string `json:"end_time"`
		Date             string `json:"date"`
		Time             string `json:"time"`
		Location         string `json:"location"`
		EventType        string `json:"event_type"`
		Type             string `json:"type"`
		IsImportant      bool   `json:"is_important"`
		RecurrenceRule   string `json:"recurrence_rule"`
		ReminderMinutes  int    `json:"reminder_minutes"`
		Color            string `json:"color"`
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

	// 校验周期规则
	if req.RecurrenceRule != "" {
		if _, err := calendar.ParseRule(req.RecurrenceRule); err != nil {
			response.BadRequest(c, "周期规则无效: "+err.Error())
			return
		}
	}

	var startTime, endTime time.Time
	if req.Date != "" && req.Time != "" {
		startTime, _ = time.Parse("2006-01-02 15:04", req.Date+" "+req.Time)
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}
	if req.EndTime != "" {
		endTime, _ = time.Parse("2006-01-02 15:04", req.Date+" "+req.EndTime)
	}
	if endTime.IsZero() {
		endTime = startTime.Add(2 * time.Hour)
	}
	if req.ReminderMinutes == 0 {
		req.ReminderMinutes = 30
	}

	_ = strconv.Atoi // 避免 unused

	e := &model.CalendarEvent{
		MemberID: req.MemberID, Title: req.Title, Description: req.Description,
		StartTime: startTime, EndTime: endTime, Location: req.Location,
		EventType: req.EventType, Date: req.Date, Time: req.Time, Type: req.Type,
		IsImportant: req.IsImportant, RecurrenceRule: req.RecurrenceRule,
		ReminderMinutes: req.ReminderMinutes, CreatedBy: 0, Color: req.Color, // 0 = 系统
	}
	id, err := db.CreateCalendarEvent(c.Request.Context(), e)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "事件创建成功"})
}
