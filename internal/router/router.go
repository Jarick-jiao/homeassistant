package router

import (
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/handler/auth"
	"github.com/homemate/server/internal/handler/calendar"
	"github.com/homemate/server/internal/handler/chat"
	"github.com/homemate/server/internal/handler/chorse"
	"github.com/homemate/server/internal/handler/dashboard"
	"github.com/homemate/server/internal/handler/feedback"
	"github.com/homemate/server/internal/handler/health"
	"github.com/homemate/server/internal/handler/iot"
	"github.com/homemate/server/internal/handler/member"
	"github.com/homemate/server/internal/handler/points"
	"github.com/homemate/server/internal/handler/records"
	"github.com/homemate/server/internal/handler/trip"
	"github.com/homemate/server/internal/handler/wechat"
	"github.com/homemate/server/internal/handler/weekend"
	"github.com/homemate/server/internal/router/middleware"
	"github.com/homemate/server/internal/service/scheduler"
	"github.com/homemate/server/internal/store"
)

// dbInject 将 DB 实例注入到 gin.Context（供各 handler 通过 c.Get("db") 获取）
func dbInject(db *store.DB, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Set("jwtSecret", jwtSecret)
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
	r.Use(dbInject(db, jwtSecret))

	// 静态文件服务（前端）
	r.Static("/assets", "./web/assets")
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/favicon.ico", "./web/favicon.ico")

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
	authed.DELETE("/health/metrics/:id", health.DeleteMetricHandler)

	// --- 健康档案（文件上传/管理/AI分析） ---
	recordsHandler := records.New(db, "")
	authed.POST("/records/upload", recordsHandler.Upload)
	authed.GET("/records", recordsHandler.List)
	authed.GET("/records/:id", recordsHandler.GetDetail)
	authed.GET("/records/:id/download", recordsHandler.Download)
	authed.DELETE("/records/:id", recordsHandler.Delete)
	authed.POST("/records/:id/analyze", recordsHandler.Analyze)
	authed.POST("/records/analyze/batch", recordsHandler.AnalyzeBatch)
	authed.GET("/records/reports", recordsHandler.ListReports)
	authed.POST("/records/reports/generate", recordsHandler.GenerateReport)

	// --- 日历 ---
	authed.GET("/calendar/events", calendar.ListCalendarEventsHandler)
	authed.POST("/calendar/events", calendar.CreateCalendarEventHandler)

	// --- 家务任务 ---
	authed.GET("/chorse/claims", chorse.GetPendingClaimsHandler)
	authed.POST("/chorse/claims", chorse.ClaimChorseHandler)
	authed.PUT("/chorse/claims/:id/complete", chorse.CompleteChorseHandler)
	authed.PUT("/chorse/claims/:id/confirm", chorse.ConfirmChorseHandler)

	// --- 积分 ---
	authed.GET("/points/dashboard", points.GetPointsDashboardHandler)

	// --- 周末出行 ---
	authed.GET("/weekend/dashboard", weekend.GetWeekendDashboardHandler)
	authed.POST("/weekend/proposals", weekend.AddProposalHandler)
	authed.POST("/weekend/vote", weekend.VoteProposalHandler)
	authed.DELETE("/weekend/vote", weekend.CancelVoteHandler)
	authed.POST("/weekend/confirm", weekend.ConfirmPlanHandler)
	authed.POST("/weekend/import-csv", weekend.ImportCSVHandler)
	authed.GET("/weekend/csv-template", weekend.GenerateCSVTemplateHandler)

	// --- 家庭成员 ---
	authed.GET("/members", member.ListMembersHandler)
	authed.GET("/members/:id", member.GetMemberDetailHandler)
	authed.POST("/members", member.CreateMemberHandler)
	authed.PUT("/members/:id", member.UpdateMemberHandler)
	authed.DELETE("/members/:id", member.DeleteMemberHandler)

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

	// --- 微信 ---
	api.POST("/wechat/callback", wechat.CallbackHandler)
	api.POST("/wechat/bind", wechat.BindHandler)

	// --- 调度器状态（管理员） ---
	adminGroup := authed.Group("")
	adminGroup.Use(middleware.RequireRole("admin"))
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

	return r
}

// secretProvider 实现 JWTSecretHolder 接口
type secretProvider struct {
	secret string
}

func (s *secretProvider) GetJWTSecret() string {
	return s.secret
}