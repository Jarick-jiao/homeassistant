package router

import (
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/handler/anniversary"
	"github.com/homemate/server/internal/handler/archive"
	"github.com/homemate/server/internal/handler/auth"
	"github.com/homemate/server/internal/handler/calendar"
	"github.com/homemate/server/internal/handler/chat"
	"github.com/homemate/server/internal/handler/chorse"
	"github.com/homemate/server/internal/handler/dashboard"
	"github.com/homemate/server/internal/handler/feedback"
	"github.com/homemate/server/internal/handler/health"
	"github.com/homemate/server/internal/handler/iot"
	"github.com/homemate/server/internal/handler/member"
	"github.com/homemate/server/internal/handler/messageboard"
	"github.com/homemate/server/internal/handler/news"
	"github.com/homemate/server/internal/handler/notification"
	"github.com/homemate/server/internal/handler/points"
	"github.com/homemate/server/internal/handler/records"
	"github.com/homemate/server/internal/handler/reminder"
	"github.com/homemate/server/internal/handler/trip"
	"github.com/homemate/server/internal/handler/wechat"
	"github.com/homemate/server/internal/handler/weekend"
	"github.com/homemate/server/internal/router/middleware"
	"github.com/homemate/server/internal/service/scheduler"
	"github.com/homemate/server/internal/store"
)

// dbInject 将 DB 实例注入到 gin.Context（供各 handler 通过 c.Get("db") 获取）
func dbInject(db *store.DB, jwtSecret string, sched *scheduler.Scheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Set("jwtSecret", jwtSecret)
		if sched != nil {
			c.Set("scheduler", sched)
		}
		c.Next()
	}
}

// Setup 配置并返回 Gin 引擎
func Setup(db *store.DB, sched *scheduler.Scheduler, jwtSecret string, serverMode string) *gin.Engine {
	gin.SetMode(serverMode)
	r := gin.New()

	// 全局中间件
	r.Use(gin.Recovery())
	r.Use(middleware.LoggerMiddleware())
	r.Use(middleware.CORSMiddleware())
	r.Use(dbInject(db, jwtSecret, sched))

	// 静态文件服务（前端）
	r.Static("/assets", "./web/assets")
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/favicon.ico", "./web/favicon.ico")
	// PWA: manifest + service worker
	r.StaticFile("/manifest.json", "./web/manifest.json")
	r.StaticFile("/sw.js", "./web/sw.js")

	// 文件上传下载的静态服务
	r.Static("/uploads", "./uploads")

	api := r.Group("/api")

	// ============ 公开路由（无需 JWT） ============
	api.POST("/auth/register", auth.RegisterHandler)
	api.POST("/auth/login", auth.LoginHandler)

	// 家务面板（公开，前端大屏展示用）
	api.GET("/chorse/dashboard", chorse.GetChorseDashboardHandler)

	// ============ 需要认证的路由 ============
	authed := api.Group("")
	authed.Use(middleware.AuthMiddleware(&secretProvider{secret: jwtSecret}))

	// --- 认证 ---
	authed.POST("/auth/reset-password", auth.ResetPasswordHandler)

	// --- 健康数据 ---
	authed.GET("/health/overview", health.GetHealthSummaryHandler)
	authed.GET("/health/records", health.ListHealthRecordsHandler)
	authed.POST("/health/records", health.CreateHealthRecordHandler)
	authed.GET("/health/real-data", health.GetRealHealthDataHandler)
	authed.POST("/health/sync", health.SyncHealthDataHandler)
	authed.GET("/health/data-source/configs", health.GetDataSourceConfigsHandler)
	authed.POST("/health/data-source/config", health.SaveDataSourceConfigHandler)

	// --- 健康指标（自定义） ---
	authed.GET("/health/metrics", health.ListMetricsHandler)
	authed.POST("/health/metrics", health.AddMetricHandler)
	authed.DELETE("/health/metrics/:id", middleware.RequireAdmin(), health.DeleteMetricHandler)

	// --- 健康档案（文件上传/管理/AI分析） ---
	recordsHandler := records.New(db, "")
	authed.POST("/records/upload", recordsHandler.Upload)
	authed.GET("/records", recordsHandler.List)
	authed.GET("/records/:id", recordsHandler.GetDetail)
	authed.GET("/records/:id/download", recordsHandler.Download)
	authed.DELETE("/records/:id", middleware.RequireAdmin(), recordsHandler.Delete)
	authed.POST("/records/:id/analyze", recordsHandler.Analyze)
	authed.POST("/records/analyze/batch", recordsHandler.AnalyzeBatch)
	authed.GET("/records/reports", recordsHandler.ListReports)
	authed.POST("/records/reports/generate", recordsHandler.GenerateReport)

	// --- 日历 ---
	authed.GET("/calendar/events", calendar.ListCalendarEventsHandler)
	authed.POST("/calendar/events", calendar.CreateCalendarEventHandler)
	authed.PUT("/calendar/events/:id", calendar.UpdateCalendarEventHandler)
	authed.DELETE("/calendar/events/:id", middleware.RequireAdmin(), calendar.DeleteCalendarEventHandler)
	authed.GET("/calendar/upcoming", calendar.GetUpcomingEventsHandler)
	authed.DELETE("/calendar/events/all", middleware.RequireAdmin(), calendar.DeleteAllEventsHandler)
	authed.POST("/calendar/demo", middleware.RequireAdmin(), calendar.SeedDemoEventsHandler)
	authed.POST("/calendar/import", middleware.RequireAdmin(), calendar.ImportCSVHandler)

	// --- 家务任务 ---
	authed.GET("/chorse/claims", chorse.GetPendingClaimsHandler)
	authed.POST("/chorse/claims", chorse.ClaimChorseHandler)
	authed.PUT("/chorse/claims/:id/complete", chorse.CompleteChorseHandler)
	authed.PUT("/chorse/claims/:id/confirm", chorse.ConfirmChorseHandler)

	// --- 积分 ---
	authed.GET("/points/dashboard", points.GetPointsDashboardHandler)
	authed.GET("/points/weekly-goal", points.GetWeeklyGoalHandler)
	authed.PUT("/points/weekly-goal", middleware.RequireAdmin(), points.UpdateWeeklyGoalHandler)
	authed.DELETE("/points/records/:id", middleware.RequireAdmin(), points.DeletePointsRecordHandler)

	// --- 休闲活动（原周末出行） ---
	authed.GET("/weekend/dashboard", weekend.GetWeekendDashboardHandler)
	authed.POST("/weekend/proposals", weekend.AddProposalHandler)
	authed.GET("/weekend/proposals/:id", weekend.GetProposalHandler)
	authed.PUT("/weekend/proposals/:id", weekend.UpdateProposalHandler)
	authed.DELETE("/weekend/proposals/:id", middleware.RequireAdmin(), weekend.DeleteProposalHandler)
	authed.POST("/weekend/vote", weekend.VoteProposalHandler)
	authed.DELETE("/weekend/vote", weekend.CancelVoteHandler)
	authed.POST("/weekend/confirm", weekend.ConfirmPlanHandler)
	authed.POST("/weekend/import-csv", middleware.RequireAdmin(), weekend.ImportCSVHandler)
	authed.GET("/weekend/csv-template", weekend.GenerateCSVTemplateHandler)

	// --- 家庭成员 ---
	authed.GET("/members", member.ListMembersHandler)
	authed.GET("/members/:id", member.GetMemberDetailHandler)
	authed.POST("/members", middleware.RequireAdmin(), member.CreateMemberHandler)
	authed.PUT("/members/:id", middleware.RequireAdmin(), member.UpdateMemberHandler)
	authed.PUT("/members/:id/role", middleware.RequireAdmin(), member.UpdateMemberRoleHandler)
	authed.DELETE("/members/:id", middleware.RequireAdmin(), member.DeleteMemberHandler)

	// --- 留言板 ---
	authed.GET("/messages", messageboard.ListMessagesHandler)
	authed.POST("/messages", messageboard.CreateMessageHandler)
	authed.GET("/messages/:id", messageboard.GetMessageHandler)
	authed.PUT("/messages/:id/read", messageboard.MarkMessageReadHandler)
	authed.DELETE("/messages/:id", middleware.RequireAdmin(), messageboard.DeleteMessageHandler)
	authed.PUT("/messages/:id/pin", middleware.RequireAdmin(), messageboard.PinMessageHandler)

	// --- 通知中心 ---
	authed.GET("/notifications", notification.ListNotificationsHandler)
	authed.GET("/notifications/unread-count", notification.UnreadCountHandler)
	authed.PUT("/notifications/:id/read", notification.MarkReadHandler)
	authed.PUT("/notifications/read-all", notification.MarkAllReadHandler)

	// --- 提醒 ---
	authed.GET("/reminders", reminder.ListRemindersHandler)
	authed.POST("/reminders", reminder.CreateReminderHandler)
	authed.PUT("/reminders/:id", reminder.UpdateReminderHandler)
	authed.DELETE("/reminders/:id", middleware.RequireAdmin(), reminder.DeleteReminderHandler)

	// --- Dashboard ---
	authed.GET("/dashboard", dashboard.GetDashboardHandler)
	api.GET("/dashboard/bigscreen", dashboard.GetBigScreenHandler)

	// --- 聊天 ---
	api.GET("/chat/ws", chat.WebSocketHandler)
	authed.POST("/chat", chat.ChatHandler)

	// --- IoT 设备 ---
	authed.GET("/iot/devices", iot.ListIoTDevicesHandler)
	authed.POST("/iot/devices/control", iot.ControlIoTDeviceHandler)

	// --- 反馈 ---
	authed.GET("/feedback", feedback.ListFeedbackHandler)
	authed.POST("/feedback", feedback.SubmitFeedbackHandler)

	// --- 出行计划 ---
	authed.GET("/trips", trip.ListTripsHandler)
	authed.POST("/trips", trip.CreateTripHandler)

	// --- 时事新闻（公开读取） ---
	api.GET("/news", news.ListNewsHandler)
	authed.DELETE("/news/:id", middleware.RequireAdmin(), news.DeleteNewsHandler)
	authed.DELETE("/news/all", middleware.RequireAdmin(), news.DeleteAllNewsHandler)
	authed.POST("/news/demo", middleware.RequireAdmin(), news.SeedDemoNewsHandler)
	authed.POST("/news/import-csv", middleware.RequireAdmin(), news.ImportCSVHandler)
	authed.GET("/news/csv-template", middleware.RequireAdmin(), news.GenerateCSVTemplateHandler)

	// --- 纪念日（公开读取） ---
	api.GET("/anniversaries", anniversary.ListAnniversariesHandler)
	api.GET("/anniversaries/upcoming", anniversary.GetUpcomingAnniversariesHandler)
	authed.POST("/anniversaries", anniversary.CreateAnniversaryHandler)
	authed.PUT("/anniversaries/:id", anniversary.UpdateAnniversaryHandler)
	authed.DELETE("/anniversaries/:id", anniversary.DeleteAnniversaryHandler)

	// --- API Token 管理（管理员） ---
	adminGroup := authed.Group("")
	adminGroup.Use(middleware.RequireRole("admin"))
	adminGroup.GET("/tokens", auth.ListAPITokensHandler)
	adminGroup.POST("/tokens", auth.CreateAPITokenHandler)
	adminGroup.DELETE("/tokens/:id", auth.DeleteAPITokenHandler)
	// 归档查询（v3.3.1）
	adminGroup.GET("/archive", archive.ListArchiveTablesHandler)
	adminGroup.GET("/archive/:table", archive.ListArchivedHandler)

	// --- 外部数据写入接口（需 API Token） ---
	external := api.Group("/external")
	external.Use(dbInject(db, jwtSecret, sched))
	external.POST("/news", middleware.APITokenAuth("news:write"), news.CreateNewsHandler)
	external.POST("/news/batch", middleware.APITokenAuth("news:write"), news.BatchCreateNewsHandler)
	external.POST("/anniversaries", middleware.APITokenAuth("anniversary:write"), anniversary.CreateAnniversaryHandler)
	external.PUT("/anniversaries/:id", middleware.APITokenAuth("anniversary:write"), anniversary.UpdateAnniversaryHandler)
	external.DELETE("/anniversaries/:id", middleware.APITokenAuth("anniversary:write"), anniversary.DeleteAnniversaryHandler)
	external.POST("/weekend/proposals", middleware.APITokenAuth("weekend:write"), weekend.ExternalAddProposalHandler)
	external.POST("/calendar/events", middleware.APITokenAuth("calendar:write"), calendar.ExternalCreateEventHandler)

	// --- 微信 ---
	api.POST("/wechat/callback", wechat.CallbackHandler)
	api.POST("/wechat/bind", wechat.BindHandler)

	// --- 调度器状态（管理员） ---
	adminGroup.GET("/scheduler/status", func(c *gin.Context) {
		if sched == nil {
			c.JSON(200, gin.H{"running": false, "tasks": []interface{}{}})
			return
		}
		c.JSON(200, sched.Status())
	})
	adminGroup.POST("/scheduler/trigger/:task", func(c *gin.Context) {
		if sched == nil {
			c.JSON(500, gin.H{"code": 500, "message": "调度器未启动"})
			return
		}
		task := c.Param("task")
		if err := sched.TriggerManual(task); err != nil {
			c.JSON(400, gin.H{"code": 400, "message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "message": "任务已触发: " + task})
	})
	adminGroup.GET("/chorse/tasks", chorse.ListAllChorseTasksHandler)
	adminGroup.PUT("/chorse/tasks/toggle", chorse.ToggleChorseTaskHandler)
	// --- 微信机器人（管理员配置） ---
	adminGroup.PUT("/wechat/test-push", wechat.TestPushHandler)
	adminGroup.PUT("/wecom/config", wechat.UpdateWeComConfigHandler)
	adminGroup.GET("/wecom/config", wechat.GetWeComConfigHandler)

	return r
}

// secretProvider 实现 JWTSecretHolder 接口
type secretProvider struct {
	secret string
}

func (s *secretProvider) GetJWTSecret() string {
	return s.secret
}