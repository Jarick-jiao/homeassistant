package calendar

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/service/calendar"
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
	// 通过 user_id 反查 member_id
	userIDVal, _ := c.Get("userID")
	userID, _ := userIDVal.(int64)
	db := getDB(c)
	if db == nil || userID == 0 {
		return 0
	}
	memberID, err := db.GetMemberIDByUserID(c.Request.Context(), userID)
	if err != nil {
		return 0
	}
	return memberID
}

// ListCalendarEventsHandler 列出日历事件（支持日期范围 + 周期展开）
func ListCalendarEventsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	from := c.Query("from")
	to := c.Query("to")
	important := c.Query("important")

	var events []model.CalendarEvent
	var err error
	if from != "" && to != "" {
		events, err = db.ListCalendarEventsByDateRange(c.Request.Context(), from, to)
	} else {
		events, err = db.GetEvents(c.Request.Context(), 0)
	}
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}

	// 对有周期规则的事件展开实例
	var expanded []model.CalendarEvent
	now := time.Now()
	rangeFrom := now.AddDate(0, -1, 0)
	rangeTo := now.AddDate(0, 2, 0)
	if from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			rangeFrom = t
		}
	}
	if to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			rangeTo = t.Add(24 * time.Hour)
		}
	}
	for _, e := range events {
		if important == "1" && !e.IsImportant {
			continue
		}
		if e.RecurrenceRule == "" {
			expanded = append(expanded, e)
			continue
		}
		// 展开周期实例
		rule, err := calendar.ParseRule(e.RecurrenceRule)
		if err != nil || rule == nil {
			expanded = append(expanded, e)
			continue
		}
		if e.StartTime.IsZero() {
			expanded = append(expanded, e)
			continue
		}
		instances, err := calendar.ExpandOccurrences(e.StartTime, *rule, rangeFrom, rangeTo)
		if err != nil || len(instances) == 0 {
			expanded = append(expanded, e)
			continue
		}
		for _, inst := range instances {
			copy := e
			copy.StartTime = inst
			copy.EndTime = inst.Add(e.EndTime.Sub(e.StartTime))
			expanded = append(expanded, copy)
		}
	}
	response.Success(c, expanded)
}

// GetUpcomingEventsHandler 返回未来 7 天事件
func GetUpcomingEventsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []interface{}{})
		return
	}
	now := time.Now()
	to := now.AddDate(0, 0, 7)
	events, err := db.ListCalendarEventsByDateRange(c.Request.Context(), now.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		response.Success(c, []interface{}{})
		return
	}
	// 展开周期
	var expanded []model.CalendarEvent
	for _, e := range events {
		if e.RecurrenceRule == "" {
			expanded = append(expanded, e)
			continue
		}
		rule, err := calendar.ParseRule(e.RecurrenceRule)
		if err != nil || rule == nil || e.StartTime.IsZero() {
			expanded = append(expanded, e)
			continue
		}
		instances, err := calendar.ExpandOccurrences(e.StartTime, *rule, now, to)
		if err != nil || len(instances) == 0 {
			continue
		}
		for _, inst := range instances {
			copy := e
			copy.StartTime = inst
			expanded = append(expanded, copy)
		}
	}
	response.Success(c, expanded)
}

// CreateCalendarEventHandler 创建日历事件（含扩展字段）
func CreateCalendarEventHandler(c *gin.Context) {
	var req struct {
		MemberID        int64  `json:"member_id"`
		Title           string `json:"title" binding:"required"`
		Description     string `json:"description"`
		StartTime       string `json:"start_time"`
		EndTime         string `json:"end_time"`
		Date            string `json:"date"`
		Time            string `json:"time"`
		Location        string `json:"location"`
		EventType       string `json:"event_type"`
		Type            string `json:"type"`
		IsImportant     bool   `json:"is_important"`
		RecurrenceRule  string `json:"recurrence_rule"`
		ReminderMinutes int    `json:"reminder_minutes"`
		Color           string `json:"color"`
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
	createdBy := getCurrentMemberID(c)
	if req.MemberID == 0 {
		req.MemberID = createdBy
	}

	e := &model.CalendarEvent{
		MemberID: req.MemberID, Title: req.Title, Description: req.Description,
		StartTime: startTime, EndTime: endTime, Location: req.Location,
		EventType: req.EventType, Date: req.Date, Time: req.Time, Type: req.Type,
		IsImportant: req.IsImportant, RecurrenceRule: req.RecurrenceRule,
		ReminderMinutes: req.ReminderMinutes, CreatedBy: createdBy, Color: req.Color,
	}
	id, err := db.CreateCalendarEvent(c.Request.Context(), e)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "事件创建成功"})
}

// UpdateCalendarEventHandler 更新事件
func UpdateCalendarEventHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "ID 格式错误")
		return
	}
	var req struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		StartTime       string `json:"start_time"`
		EndTime         string `json:"end_time"`
		Date            string `json:"date"`
		Time            string `json:"time"`
		Location        string `json:"location"`
		EventType       string `json:"event_type"`
		Type            string `json:"type"`
		IsImportant     bool   `json:"is_important"`
		RecurrenceRule  string `json:"recurrence_rule"`
		ReminderMinutes int    `json:"reminder_minutes"`
		Color           string `json:"color"`
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
	existing, err := db.GetCalendarEventByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "事件不存在")
		return
	}
	// 作者/admin 校验
	currentMember := getCurrentMemberID(c)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(model.Role)
	if existing.CreatedBy != 0 && existing.CreatedBy != currentMember && role != model.RoleAdmin {
		response.Forbidden(c, "只能修改自己创建的事件")
		return
	}

	if req.RecurrenceRule != "" {
		if _, err := calendar.ParseRule(req.RecurrenceRule); err != nil {
			response.BadRequest(c, "周期规则无效: "+err.Error())
			return
		}
	}
	if req.Title != "" {
		existing.Title = req.Title
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Date != "" {
		existing.Date = req.Date
	}
	if req.Time != "" {
		existing.Time = req.Time
	}
	if req.Location != "" {
		existing.Location = req.Location
	}
	if req.EventType != "" {
		existing.EventType = req.EventType
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	existing.IsImportant = req.IsImportant
	if req.RecurrenceRule != "" || req.RecurrenceRule == "" {
		existing.RecurrenceRule = req.RecurrenceRule
	}
	if req.ReminderMinutes > 0 {
		existing.ReminderMinutes = req.ReminderMinutes
	}
	if req.Color != "" {
		existing.Color = req.Color
	}
	if req.Date != "" && req.Time != "" {
		if st, err := time.Parse("2006-01-02 15:04", req.Date+" "+req.Time); err == nil {
			existing.StartTime = st
		}
	}
	if err := db.UpdateCalendarEvent(c.Request.Context(), existing); err != nil {
		response.InternalServerError(c, "更新失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "事件已更新"})
}

// DeleteCalendarEventHandler 删除事件（admin 或作者）
func DeleteCalendarEventHandler(c *gin.Context) {
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
	existing, err := db.GetCalendarEventByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, 404, "事件不存在")
		return
	}
	currentMember := getCurrentMemberID(c)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(model.Role)
	if existing.CreatedBy != 0 && existing.CreatedBy != currentMember && role != model.RoleAdmin {
		response.Forbidden(c, "只能删除自己创建的事件")
		return
	}
	if err := db.DeleteCalendarEvent(c.Request.Context(), id); err != nil {
		response.InternalServerError(c, "删除失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true, "id": id})
}
