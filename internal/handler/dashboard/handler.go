package dashboard

import (
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/handler/chorse"
	"github.com/homemate/server/internal/handler/points"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// GetDashboardHandler 获取角色专属的 Dashboard 数据
// 安全修复（QA C-10）：不再从 URL 参数获取角色，而是从 JWT claims 中获取
func GetDashboardHandler(c *gin.Context) {
	roleVal, exists := c.Get("role")
	if !exists {
		response.BadRequest(c, "无法获取用户角色信息")
		return
	}
	role, ok := roleVal.(model.Role)
	if !ok {
		response.BadRequest(c, "用户角色类型错误")
		return
	}
	_ = c.Param("role") // 忽略 URL 参数，使用 JWT 中的角色

	userID, _ := c.Get("userID")
	familyID, _ := c.Get("familyID")
	username, _ := c.Get("username")
	_ = userID

	var data interface{}

	switch role {
	case model.RoleAdmin:
		data = gin.H{
			"role":      "admin",
			"username":  username,
			"family_id": familyID,
			"members_summary": []gin.H{
				{"id": 1, "name": "爸爸", "role": "adult", "status": "online"},
				{"id": 2, "name": "妈妈", "role": "adult", "status": "online"},
				{"id": 3, "name": "小明", "role": "child", "status": "offline"},
				{"id": 4, "name": "爷爷", "role": "elder", "status": "online"},
			},
			"total_devices": 8,
			"unread_alerts": 2,
			"system_status": "running",
			"quick_actions": []string{"添加成员", "设备管理", "查看日志"},
		}

	case model.RoleAdult:
		data = gin.H{
			"role":      "adult",
			"username":  username,
			"family_id": familyID,
			"my_data": gin.H{
				"tasks_today":     5,
				"messages_unread": 3,
				"upcoming_events": 2,
			},
			"children_data": []gin.H{
				{"child_id": 3, "name": "小明", "location": "学校", "homework_done": true, "mood": "happy"},
			},
			"elder_data": []gin.H{
				{"elder_id": 4, "name": "爷爷", "health_status": "good", "medication_reminder": true},
			},
			"quick_actions": []string{"查看监控", "设置提醒", "健康数据", "一键呼叫"},
		}

	case model.RoleChild:
		data = gin.H{
			"role":            "child",
			"username":        username,
			"simplified_view": true,
			"widgets": []gin.H{
				{"type": "schedule", "title": "今日课程", "items": []string{"语文", "数学", "英语"}},
				{"type": "task", "title": "待完成作业", "items": []string{"数学练习册 P12", "背古诗一首"}},
				{"type": "family", "title": "家人动态", "items": []string{"爸爸在公司", "妈妈在家"}},
			},
			"quick_actions": []string{"呼叫爸妈", "查看课表"},
		}

	case model.RoleElder:
		data = gin.H{
			"role":          "elder",
			"username":      username,
			"large_font":    true,
			"voice_support": true,
			"high_contrast": true,
			"widgets": []gin.H{
				{"type": "health", "title": "健康指标", "font_size": "xlarge", "items": []gin.H{
					{"label": "血压", "value": "120/80", "unit": "mmHg"},
					{"label": "心率", "value": "72", "unit": "bpm"},
				}},
				{"type": "medication", "title": "用药提醒", "font_size": "xlarge", "items": []string{"降压药 08:00", "钙片 12:00"}},
				{"type": "family", "title": "一键呼叫", "font_size": "xlarge", "items": []gin.H{
					{"name": "儿子", "phone": "13800138000"},
					{"name": "女儿", "phone": "13900139000"},
				}},
			},
			"emergency_contact": gin.H{
				"name":  "儿子",
				"phone": "13800138000",
			},
			"voice_hint": "点击麦克风图标可使用语音助手",
		}

	default:
		response.BadRequest(c, "未知的角色类型: "+string(role))
		return
	}

	response.Success(c, data)
}

// GetBigScreenHandler 大屏综合展示
// 从各模块的内存存储中聚合真实数据
func GetBigScreenHandler(c *gin.Context) {
	now := time.Now().Format("2006-01-02 15:04:05")

	// 提取 db 引用（函数级作用域，供各板块使用）
	var db *store.DB
	if dbVal, exists := c.Get("db"); exists && dbVal != nil {
		if d, ok := dbVal.(*store.DB); ok {
			db = d
		}
	}

	// === 1. 健康板块 ===
	healthMembers := []gin.H{}
	if db != nil {
		members, err := db.GetMembers(c.Request.Context())
		if err == nil {
			metricsMap := map[string][]gin.H{}
			allMetrics, _ := db.GetAllHealthMetrics(c.Request.Context())
			for _, m := range allMetrics {
				metricsMap[m.MemberName] = append(metricsMap[m.MemberName], gin.H{
					"icon": m.Icon, "label": m.Label, "value": m.Value,
					"unit": m.Unit, "status": m.Status, "trend": m.Trend,
				})
			}
			for _, mem := range members {
				metrics := metricsMap[mem.Name]
				if metrics == nil {
					metrics = []gin.H{}
				}
				healthMembers = append(healthMembers, gin.H{
					"name": mem.Name, "role": mem.Role,
					"status": "normal", "status_text": mem.Role + " · 正常",
					"metrics": metrics,
				})
			}
		}
	}

	// === 2. 家务板块（从 chorse 包获取真实数据）===
	chorseRecords := chorse.GetTodayRecords(c)
	chorseStats := gin.H{
		"total_points":   0,
		"completed":      len(chorseRecords),
		"active_members": 0,
		"streak":         7,
	}
	chorseMemberSet := map[string]bool{}
	chorseTotalPts := 0
	for _, r := range chorseRecords {
		chorseTotalPts += r.Points
		chorseMemberSet[r.Member] = true
	}
	chorseStats["total_points"] = chorseTotalPts
	chorseStats["active_members"] = len(chorseMemberSet)

	// 转换家务记录为大屏格式
	bigscreenChorseRecords := []gin.H{}
	for _, r := range chorseRecords {
		bigscreenChorseRecords = append(bigscreenChorseRecords, gin.H{
			"time": r.Time, "member": r.Member, "task": r.Task, "icon": r.Icon, "points": r.Points,
		})
	}

	// 成员贡献
	chorseContrib := chorse.GetMemberContrib(c)
	bigscreenContrib := []gin.H{}
	for _, contrib := range chorseContrib {
		bigscreenContrib = append(bigscreenContrib, gin.H{
			"name": contrib.Name, "done": contrib.Done, "points": contrib.Points,
		})
	}

	// === 3. 积分板块（从 points 包获取真实数据）===
	ptsMembers := points.GetMembersSnapshot(c)
	bigscreenRanking := []gin.H{}
	familyTotal := 0
	for _, m := range ptsMembers {
		familyTotal += m.Total
		bigscreenRanking = append(bigscreenRanking, gin.H{
			"name": m.Name, "total": m.Total, "level": m.Level, "rank": 0,
		})
	}
	// 按总积分排序
	sort.Slice(bigscreenRanking, func(i, j int) bool {
		iv, _ := bigscreenRanking[i]["total"].(int)
		jv, _ := bigscreenRanking[j]["total"].(int)
		return iv > jv
	})
	for i := range bigscreenRanking {
		bigscreenRanking[i]["rank"] = i + 1
	}

	// === 5. 时事新闻板块（最新 5 条）===
	newsItems := []gin.H{}
	if db != nil {
		if list, _, err := db.ListNews(c.Request.Context(), "all", 5, 0); err == nil {
			for _, n := range list {
				newsItems = append(newsItems, gin.H{
					"id":           n.ID,
					"category":     n.Category,
					"title":        n.Title,
					"summary":      n.Summary,
					"source":       n.Source,
					"image_url":    n.ImageURL,
					"published_at": n.PublishedAt.Format("01-02 15:04"),
					"is_hot":       n.IsHot,
				})
			}
		}
	}

	// === 6. 家庭日历板块（未来 7 天）===
	calendarEvents := []gin.H{}
	if db != nil {
		from := time.Now().Format("2006-01-02")
		to := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
		if events, err := db.ListCalendarEventsByDateRange(c.Request.Context(), from, to); err == nil {
			for _, e := range events {
				calendarEvents = append(calendarEvents, gin.H{
					"id":         e.ID,
					"title":      e.Title,
					"date":       e.Date,
					"time":       e.Time,
					"location":   e.Location,
					"event_type": e.EventType,
				})
			}
		}
	}

	// === 7. 纪念日板块（未来 30 天）===
	anniversaryItems := []gin.H{}
	if db != nil {
		if items, err := db.GetUpcomingAnniversaries(c.Request.Context(), 30); err == nil {
			for _, a := range items {
				anniversaryItems = append(anniversaryItems, gin.H{
					"id":         a.ID,
					"title":      a.Title,
					"date":       a.Date,
					"type":       a.Type,
					"days_until": a.DaysUntil,
					"next_date":  a.NextDate,
					"is_lunar":   a.IsLunar,
				})
			}
		}
	}

	// === 4. 活动推荐板块 ===

	// 读取周目标设置（默认 500）+ 动态计算本周进度
	weeklyGoal := 500
	var weeklyProgress float64
	if db != nil {
		weeklyGoalStr := db.GetSetting(c.Request.Context(), "weekly_goal")
		if v, err := strconv.Atoi(weeklyGoalStr); err == nil && v > 0 {
			weeklyGoal = v
		}
		weeklySum, _ := db.GetWeeklyPointsSum(c.Request.Context())
		if weeklyGoal > 0 {
			weeklyProgress = float64(weeklySum) / float64(weeklyGoal)
			if weeklyProgress > 1.0 {
				weeklyProgress = 1.0
			}
		}
	}

	data := gin.H{
		"timestamp": now,
		"sections": []gin.H{
			{
				"type":    "health",
				"title":   "家庭健康总览",
				"icon":    "❤️",
				"members": healthMembers,
				"alerts":  []gin.H{},
			},
			{
				"type":           "chores",
				"title":          "今日家务",
				"icon":           "🧹",
				"stats":          chorseStats,
				"today_records":  bigscreenChorseRecords,
				"member_contrib": bigscreenContrib,
			},
			{
				"type":      "activities",
				"title":     "活动推荐",
				"icon":      "🎯",
				"weekend":   "",
				"weather":   "",
				"proposals": []gin.H{},
			},
			{
				"type":            "points",
				"title":           "积分动态",
				"icon":            "🏆",
				"family_total":    familyTotal,
				"weekly_goal":     weeklyGoal,
				"weekly_progress": weeklyProgress,
				"ranking":         bigscreenRanking,
			},
			{
				"type":  "news",
				"title": "时事新闻",
				"icon":  "📰",
				"items": newsItems,
			},
			{
				"type":   "calendar",
				"title":  "家庭日历",
				"icon":   "📅",
				"events": calendarEvents,
			},
			{
				"type":         "anniversary",
				"title":        "纪念日",
				"icon":         "💝",
				"anniversaries": anniversaryItems,
			},
		},
	}

	response.Success(c, data)
}
