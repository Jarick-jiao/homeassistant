package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/homemate/server/internal/config"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/service/garmin"
	"github.com/homemate/server/internal/service/weather"
	botservice "github.com/homemate/server/internal/service/wechat"
	"github.com/homemate/server/internal/store"
)

// taskMeta 任务元信息（用于状态展示）
type taskMeta struct {
	Task     string        // 任务标识（用于手动触发）
	Name     string        // 中文名
	Interval time.Duration // 执行间隔（0=按条件触发，如每日定点/每周）
	Enabled  bool          // 是否启用
}

// Scheduler 定时任务调度器
type Scheduler struct {
	db            *store.DB
	dbPath        string // v3.9.8: SQLite 路径（传给 Python 脚本）
	garminClient  garmin.Client
	weatherClient weather.Client
	pusher        botservice.Pusher
	// v3.9.7: 全局 Garmin 凭证兜底（DB data_source_config 未配时使用 config/环境变量）
	garminCfg config.GarminConfig
	// v3.9.12: Apple 日历同步配置（osascript → calendar_events，仅 macOS）
	calendarSyncCfg config.CalendarSyncConfig
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
	lastRuns  map[string]time.Time // 记录各任务最后执行时间
	// v3.9.13: 任务注册表 + 上次执行状态，让 /api/scheduler/status 返回完整任务列表
	taskRegistry []taskMeta
	lastStatus   map[string]string // success / error / ""
}

// recordRun 统一记录任务执行（时间 + 状态），替代散落的 s.lastRuns[x]=time.Now()
func (s *Scheduler) recordRun(name string, err error) {
	s.mu.Lock()
	s.lastRuns[name] = time.Now()
	if err != nil {
		s.lastStatus[name] = "error"
	} else {
		s.lastStatus[name] = "success"
	}
	s.mu.Unlock()
}

// TaskConfig 任务配置
type TaskConfig struct {
	// HealthSyncCron 健康数据同步间隔（cron: 秒 分 时 日 月 周）
	// 当前使用固定间隔简化实现
	HealthSyncInterval time.Duration
	// AIAnalysisHour 每日 AI 分析时间（小时，24h 制）
	AIAnalysisHour int
	// WeekendRecommendDay 周几生成周末推荐（0=周日, 1=周一 ... 4=周四）
	WeekendRecommendWeekday time.Weekday
	// WeekendRecommendHour 周末推荐生成时间（小时）
	WeekendRecommendHour int
}

// DefaultTaskConfig 默认任务配置
func DefaultTaskConfig() TaskConfig {
	return TaskConfig{
		HealthSyncInterval:      6 * time.Hour,
		AIAnalysisHour:          8,
		WeekendRecommendWeekday: time.Thursday,
		WeekendRecommendHour:    19,
	}
}

// New 创建调度器
// garminClient/weatherClient/pusher 可为 nil（未配置时跳过对应功能）
// garminCfg 为全局 Garmin 凭证兜底（DB data_source_config 未配时使用 config/环境变量）
// dbPath 为 SQLite 数据库路径（v3.9.8: 脚本同步模式传给 Python 脚本）
// calendarSyncCfg 为 Apple 日历同步配置（v3.9.12: osascript → calendar_events，仅 macOS）
func New(db *store.DB, dbPath string, garminClient garmin.Client, weatherClient weather.Client, pusher botservice.Pusher, garminCfg config.GarminConfig, calendarSyncCfg config.CalendarSyncConfig) *Scheduler {
	if pusher == nil {
		pusher = botservice.NewPusher("")
	}
	return &Scheduler{
		db:              db,
		dbPath:          dbPath,
		garminClient:    garminClient,
		weatherClient:   weatherClient,
		pusher:          pusher,
		garminCfg:       garminCfg,
		calendarSyncCfg: calendarSyncCfg,
		stopCh:          make(chan struct{}),
		lastRuns:        make(map[string]time.Time),
		lastStatus:      make(map[string]string),
	}
}

// Start 启动调度器
func (s *Scheduler) Start(cfg TaskConfig) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Println("[SCHEDULER] 定时任务调度器已启动")
	log.Printf("[SCHEDULER] 健康数据同步间隔: %v", cfg.HealthSyncInterval)
	log.Printf("[SCHEDULER] AI 分析时间: 每天 %02d:00", cfg.AIAnalysisHour)
	log.Printf("[SCHEDULER] 周末推荐生成: 每周%s %02d:00", weekdayName(cfg.WeekendRecommendWeekday), cfg.WeekendRecommendHour)

	// v3.9.13: 构建任务注册表（供 /api/scheduler/status 展示完整列表，含未执行过的任务）
	healthEnabled := s.garminCfg.UseScriptSync || s.garminClient != nil
	s.taskRegistry = []taskMeta{
		{Task: "health_sync", Name: "健康数据同步", Interval: cfg.HealthSyncInterval, Enabled: healthEnabled},
		{Task: "calendar_sync", Name: "Apple 日历同步", Interval: s.calendarSyncCfg.Interval, Enabled: s.calendarSyncCfg.Enable},
		{Task: "ai_analysis", Name: "AI 健康分析", Interval: 24 * time.Hour, Enabled: true},
		{Task: "weekend_recommend", Name: "周末推荐生成", Interval: 7 * 24 * time.Hour, Enabled: true},
		{Task: "reminder_scan", Name: "提醒扫描", Interval: 5 * time.Minute, Enabled: true},
		{Task: "notification_push", Name: "通知推送", Interval: 1 * time.Minute, Enabled: true},
		{Task: "calendar_reminder", Name: "日程提醒", Interval: 1 * time.Minute, Enabled: true},
		{Task: "cleanup", Name: "历史数据清理", Interval: 24 * time.Hour, Enabled: true},
	}

	// 健康数据同步
	s.wg.Add(1)
	go s.healthSyncLoop(cfg.HealthSyncInterval)

	// v3.9.12: Apple 日历同步（仅 macOS，osascript → calendar_events）
	if s.calendarSyncCfg.Enable {
		interval := s.calendarSyncCfg.Interval
		if interval <= 0 {
			interval = 1 * time.Hour
		}
		s.wg.Add(1)
		go s.appleCalendarSyncLoop(interval)
		log.Printf("[SCHEDULER] Apple 日历同步间隔: %v (脚本: %s)", interval, s.calendarSyncCfg.ScriptPath)
	} else {
		log.Println("[SCHEDULER] Apple 日历同步未启用（calendar_sync.enable=false）")
	}

	// AI 分析检查
	s.wg.Add(1)
	go s.dailyAIAnalysisLoop(cfg.AIAnalysisHour)

	// 周末推荐生成
	s.wg.Add(1)
	go s.weekendRecommendLoop(cfg.WeekendRecommendWeekday, cfg.WeekendRecommendHour)

	// 提醒扫描（每 5 分钟扫描到期提醒）
	s.wg.Add(1)
	go s.reminderScanLoop()

	// 通知推送（每 1 分钟扫描未推送通知，调 WeCom webhook）
	s.wg.Add(1)
	go s.notificationPushLoop()

	// 日程提醒（每 1 分钟扫描需要提醒的事件）
	s.wg.Add(1)
	go s.calendarReminderLoop()

	// 历史数据清理（每日 03:00 执行）
	s.wg.Add(1)
	go s.cleanupLoop(3)
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	log.Println("[SCHEDULER] 定时任务调度器已停止")
}

// IsRunning 返回调度器是否在运行
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// TriggerManual 手动触发指定任务
func (s *Scheduler) TriggerManual(taskName string) error {
	switch taskName {
	case "health_sync":
		go s.runHealthSync()
		return nil
	case "ai_analysis":
		go s.runAIAnalysis()
		return nil
	case "weekend_recommend":
		go s.runWeekendRecommend()
		return nil
	case "reminder_scan":
		go s.runReminderScan()
		return nil
	case "notification_push":
		go s.runNotificationPush()
		return nil
	case "calendar_reminder":
		go s.runCalendarReminders()
		return nil
	case "calendar_sync":
		go s.runAppleCalendarSync()
		return nil
	case "cleanup":
		go s.runCleanup()
		return nil
	default:
		return fmt.Errorf("未知任务: %s", taskName)
	}
}

// ============ 提醒扫描 ============

func (s *Scheduler) reminderScanLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runReminderScan()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runReminderScan() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now()
	reminders, err := s.db.ListDueReminders(ctx, now)
	if err != nil {
		log.Printf("[SCHEDULER] 提醒扫描失败: %v", err)
		return
	}
	for _, r := range reminders {
		notif := &model.Notification{
			MemberID: r.MemberID,
			Type:     model.NotificationTypeReminder,
			Title:    "提醒: " + r.Title,
			Body:     r.Content,
		}
		if _, err := s.db.CreateNotification(ctx, notif); err != nil {
			log.Printf("[SCHEDULER] 创建提醒通知失败 (reminder=%d): %v", r.ID, err)
			continue
		}
		if err := s.db.MarkReminderSent(ctx, r.ID); err != nil {
			log.Printf("[SCHEDULER] 标记提醒已发送失败 (reminder=%d): %v", r.ID, err)
		}
		log.Printf("[SCHEDULER] 提醒触发 (reminder=%d, member=%d): %s", r.ID, r.MemberID, r.Title)
	}
}

// ============ 通知推送 ============

func (s *Scheduler) notificationPushLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runNotificationPush()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runNotificationPush() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	notifications, err := s.db.ListUnpushedNotifications(ctx, 50)
	if err != nil || len(notifications) == 0 {
		return
	}
	for _, n := range notifications {
		err := s.pusher.Push(ctx, n.Title, n.Body)
		if err != nil {
			log.Printf("[SCHEDULER] 通知推送失败 (notif=%d): %v", n.ID, err)
			// 推送失败标记为已处理避免无限重试（本期简化）
			_ = s.db.MarkNotificationFailed(ctx, n.ID)
			continue
		}
		_ = s.db.MarkNotificationPushed(ctx, n.ID)
		log.Printf("[SCHEDULER] 通知已推送 (notif=%d, member=%d): %s", n.ID, n.MemberID, n.Title)
	}
}

// ============ 日程提醒 ============

func (s *Scheduler) calendarReminderLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runCalendarReminders()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runCalendarReminders() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now()
	events, err := s.db.ListUpcomingEventsWithReminders(ctx, now, 24*time.Hour)
	if err != nil {
		log.Printf("[SCHEDULER] 日程提醒扫描失败: %v", err)
		return
	}
	for _, e := range events {
		if e.StartTime.IsZero() {
			continue
		}
		remindAt := e.StartTime.Add(-time.Duration(e.ReminderMinutes) * time.Minute)
		if now.Before(remindAt) {
			continue
		}
		if e.LastRemindedAt != nil && e.LastRemindedAt.After(remindAt) {
			continue
		}
		targetMember := e.CreatedBy
		if targetMember == 0 {
			targetMember = e.MemberID
		}
		if targetMember == 0 {
			continue
		}
		notif := &model.Notification{
			MemberID: targetMember,
			Type:     model.NotificationTypeCalendar,
			Title:    "日程提醒: " + e.Title,
			Body:     fmt.Sprintf("%s 将于 %s 开始", e.Title, e.StartTime.Format("01-02 15:04")),
			DataJSON: fmt.Sprintf(`{"event_id":%d,"start_time":"%s"}`, e.ID, e.StartTime.Format(time.RFC3339)),
		}
		if _, err := s.db.CreateNotification(ctx, notif); err != nil {
			log.Printf("[SCHEDULER] 创建日程通知失败 (event=%d): %v", e.ID, err)
			continue
		}
		if err := s.db.MarkEventReminded(ctx, e.ID, now); err != nil {
			log.Printf("[SCHEDULER] 标记事件已提醒失败 (event=%d): %v", e.ID, err)
		}
		log.Printf("[SCHEDULER] 日程提醒触发 (event=%d, member=%d): %s", e.ID, targetMember, e.Title)
	}
}

// ============ 健康数据同步 ============

func (s *Scheduler) healthSyncLoop(interval time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 首次启动延迟 1 分钟执行
	select {
	case <-time.After(1 * time.Minute):
		s.runHealthSync()
	case <-s.stopCh:
		return
	}

	for {
		select {
		case <-ticker.C:
			s.runHealthSync()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runHealthSync() {
	s.recordRun("health_sync", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Println("[SCHEDULER] 开始同步健康数据...")

	// 1. 同步天气数据（用于周末出行面板）
	if s.weatherClient != nil {
		s.syncWeatherData(ctx)
	} else {
		log.Println("[SCHEDULER] 天气客户端未配置，跳过天气同步")
	}

	today := time.Now().Format("2006-01-02")

	// v3.9.8: 脚本同步模式（绕过 Cloudflare）— 通过 exec 调用 Python 脚本，
	// 脚本用 garminconnect/garth 拉取数据并直接写 SQLite（覆盖全部 30 字段）。
	// 启用后跳过下方 Go 版 garminClient 逐成员循环（该路径被 Cloudflare 429 拦截）。
	if s.garminCfg.UseScriptSync {
		synced := s.runScriptSyncAll(ctx, today)
		log.Printf("[SCHEDULER] 健康数据同步完成（脚本模式），共同步 %d 条记录", synced)
		return
	}

	// 2. 获取所有配置了数据源的成员
	configs, err := s.db.GetDataSourceConfigs(ctx)
	if err != nil {
		log.Printf("[SCHEDULER] 获取数据源配置失败: %v", err)
		return
	}

	synced := 0

	for _, cfg := range configs {
		if !cfg.IsActive {
			continue
		}

		// 根据数据源类型同步
		switch cfg.SourceType {
		case "garmin":
			synced += s.syncGarminData(ctx, cfg, today)
		case "manual":
			// 手动录入不做自动同步
			continue
		default:
			log.Printf("[SCHEDULER] 未知数据源类型: %s (成员: %s)", cfg.SourceType, cfg.MemberName)
		}
	}

	log.Printf("[SCHEDULER] 健康数据同步完成，共同步 %d 条记录", synced)
}

// syncWeatherData 同步天气数据到周末推荐缓存
func (s *Scheduler) syncWeatherData(ctx context.Context) {
	// 默认北京 adcode，后续可从 config 读取
	adcode := "110100"
	w, err := s.weatherClient.GetWeather(ctx, adcode)
	if err != nil {
		log.Printf("[SCHEDULER] 天气同步失败: %v", err)
		return
	}
	if w == nil {
		return
	}
	// 写入 weekend_recommendation 的 weather_data 字段（最新一条）
	// 此处仅记录日志，weekend handler 调用时实时查 AMAP 更准
	log.Printf("[SCHEDULER] 天气同步成功: %s %s %s", w.City, w.Condition, w.Temperature)
}

// syncGarminData 同步 Garmin 健康数据
// cfg.UserID=用户名, cfg.APISecret=密码, cfg.APIKey=token（可空）
func (s *Scheduler) syncGarminData(ctx context.Context, cfg model.DataSourceConfig, date string) int {
	if s.garminClient == nil {
		log.Printf("[SCHEDULER] Garmin 客户端未配置，跳过 (member=%d)", cfg.MemberID)
		return 0
	}
	memberID := cfg.MemberID
	username := cfg.UserID
	password := cfg.APISecret

	// v3.9.7: DB 凭证为空时，用全局 config.Garmin（环境变量）兜底
	if username == "" {
		username = s.garminCfg.Username
	}
	if password == "" {
		password = s.garminCfg.Password
	}

	if username == "" || password == "" {
		log.Printf("[SCHEDULER] Garmin 凭证不完整 (member=%d, db_user=%s, cfg_user=%s) - 请在 DB data_source_config 或环境变量 GARMIN_USERNAME/GARMIN_PASSWORD 配置",
			memberID, cfg.UserID, s.garminCfg.Username)
		return 0
	}

	// 检查今天是否已同步
	existing, err := s.db.GetHealthDataCache(ctx, memberID, date)
	if err == nil && existing != nil && existing.Source == "garmin" && existing.Steps > 0 {
		log.Printf("[SCHEDULER] 今日已同步 Garmin 数据 (member=%d)", memberID)
		return 0
	}

	// 登录 Garmin
	if err := s.garminClient.Login(ctx, username, password); err != nil {
		log.Printf("[SCHEDULER] Garmin 登录失败 (member=%d): %v", memberID, err)
		// 登录失败保留旧 cache，不写空值
		return 0
	}

	// 获取当日健康数据
	health, err := s.garminClient.GetDailyHealth(ctx, date)
	if err != nil {
		log.Printf("[SCHEDULER] Garmin 获取数据失败 (member=%d, date=%s): %v", memberID, date, err)
		return 0
	}

	// 映射到 HealthDataCache
	cache := &model.HealthDataCache{
		MemberID:    memberID,
		Date:        date,
		Source:      "garmin",
		Steps:       health.Steps,
		HeartRate:   health.HeartRate,
		SleepHours:  health.SleepHours,
		SleepScore:  health.SleepScore,
		Stress:      health.Stress,
		SpO2:        health.SpO2,
		BodyBattery: health.BodyBattery,
		Calories:    health.Calories,
		SyncedAt:    time.Now().Format(time.RFC3339),
	}

	if err := s.db.UpsertHealthDataCache(ctx, cache); err != nil {
		log.Printf("[SCHEDULER] 保存 Garmin 数据失败 (member=%d): %v", memberID, err)
		return 0
	}

	log.Printf("[SCHEDULER] Garmin 数据同步完成 (member=%d, date=%s, steps=%d, hr=%d)",
		memberID, date, health.Steps, health.HeartRate)
	return 1
}

// runScriptSyncAll v3.9.8: 脚本同步模式入口
// 通过 exec 调用 homemate_health_sync.py 拉取 Garmin 数据并直接写 SQLite。
// 脚本内部负责：读取 data_source_config 凭证、逐成员登录（token 缓存）、
// 聚合 4 个 Garmin 端点、写入 health_data_cache 全部 30 字段。
// 返回同步成功的成员数（从脚本输出 "成功: N" 解析，解析失败时按退出码判定）。
func (s *Scheduler) runScriptSyncAll(ctx context.Context, date string) int {
	scriptPath := s.garminCfg.SyncScriptPath
	if scriptPath == "" {
		scriptPath = "./scripts/homemate_health_sync.py"
	}
	pythonPath := s.garminCfg.PythonPath
	if pythonPath == "" {
		pythonPath = "/usr/bin/python3"
	}

	// 脚本路径转绝对路径，避免依赖 Go 进程 cwd
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		log.Printf("[SCHEDULER] 脚本路径解析失败 (%s): %v", scriptPath, err)
		absScript = scriptPath
	}
	if _, err := os.Stat(absScript); err != nil {
		log.Printf("[SCHEDULER] Garmin 同步脚本不存在: %s (%v)", absScript, err)
		return 0
	}

	// DB 路径转绝对路径（与 Go 服务写同一数据库）
	absDB := s.dbPath
	if abs, err := filepath.Abs(s.dbPath); err == nil {
		absDB = abs
	}

	// 脚本执行超时（独立于调度器 5 分钟上限，给 Garmin 登录+多成员拉取留余量）
	timeout := s.garminCfg.ScriptTimeout
	if timeout == 0 {
		timeout = 3 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 构造命令：python3 <script> --date <date>（脚本自动同步所有 active 成员）
	cmd := exec.CommandContext(runCtx, pythonPath, absScript, "--date", date)
	cmd.Dir = filepath.Dir(absScript)
	// 环境变量：继承父进程，剥离 PYTHONHOME/PYTHONPATH（避免 conda/venv 污染，
	// 对齐用户本地 `env -u PYTHONHOME -u PYTHONPATH /usr/bin/python3` 用法），
	// 注入 HOMEMATE_DB 让脚本写同一数据库，注入 GARMIN 凭证兜底。
	cmd.Env = s.buildScriptEnv(absDB)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("[SCHEDULER] 调用 Garmin 同步脚本: %s %s --date %s", pythonPath, absScript, date)

	if err := cmd.Run(); err != nil {
		log.Printf("[SCHEDULER] Garmin 同步脚本执行失败: %v", err)
		if stderr.Len() > 0 {
			log.Printf("[SCHEDULER] 脚本 stderr: %s", strings.TrimSpace(stderr.String()))
		}
		return 0
	}

	// 输出透传到日志便于排查
	out := stdout.String()
	if len(out) > 0 {
		// 仅打印最后 ~1KB 避免日志爆炸
		tail := out
		if len(tail) > 1024 {
			tail = "..." + tail[len(tail)-1024:]
		}
		log.Printf("[SCHEDULER] 脚本输出:\n%s", strings.TrimSpace(tail))
	}
	// 脚本退出码为 0 但 stderr 可能有 [ERR] 详细错误（登录失败等被脚本内部捕获，
	// 退出码仍为 0 以输出计数）。透传 stderr 便于排查 partial failure。
	if stderr.Len() > 0 {
		tail := stderr.String()
		if len(tail) > 2048 {
			tail = "..." + tail[len(tail)-2048:]
		}
		log.Printf("[SCHEDULER] 脚本 stderr:\n%s", strings.TrimSpace(tail))
	}

	// 解析 "成功: N" 提取同步条数
	return parseSyncedCount(out)
}

// buildScriptEnv 构造脚本执行环境变量
func (s *Scheduler) buildScriptEnv(absDB string) []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, e := range os.Environ() {
		// 剥离可能污染系统 python 的变量
		if strings.HasPrefix(e, "PYTHONHOME=") || strings.HasPrefix(e, "PYTHONPATH=") {
			continue
		}
		env = append(env, e)
	}
	// 注入数据库路径（脚本默认 DB_PATH 可能与服务端 config 不一致）
	env = append(env, "HOMEMATE_DB="+absDB)
	// 注入全局 Garmin 凭证兜底（DB data_source_config 未配时脚本可用）
	if s.garminCfg.Username != "" {
		env = append(env, "GARMIN_USERNAME="+s.garminCfg.Username)
	}
	if s.garminCfg.Password != "" {
		env = append(env, "GARMIN_PASSWORD="+s.garminCfg.Password)
	}
	return env
}

// parseSyncedCount 从脚本输出解析 "成功: N"
func parseSyncedCount(out string) int {
	// 匹配 "成功: 3" 或 "成功:3"
	idx := strings.LastIndex(out, "成功:")
	if idx < 0 {
		return 1 // 无法解析但脚本退出码为 0，视为整体成功
	}
	rest := strings.TrimSpace(out[idx+len("成功:"):])
	n := 0
	parsed := false
	for _, r := range rest {
		if r < '0' || r > '9' {
			if parsed {
				break
			}
			continue // 跳过数字前的空白/分隔符
		}
		parsed = true
		n = n*10 + int(r-'0')
	}
	if !parsed {
		return 1 // "成功:" 后无数字，视为整体成功
	}
	return n // 显式返回解析值（含 0：登录失败时 success=0 不应误报为 1）
}

// ============ Apple 日历同步 (v3.9.12) ============
//
// 通过 exec 调用 homemate_calendar_sync.py，脚本用 osascript 读取 macOS Calendar.app
// 中 ±N 天事件，写入 calendar_events 表（source='apple_calendar'）。事件按
// (source_account, external_event_id) 唯一索引去重，循环事件由 python-dateutil 展开。
// 静默保存：仅打印日志，不生成通知/报告。

// appleCalendarSyncLoop Apple 日历定时同步循环
func (s *Scheduler) appleCalendarSyncLoop(interval time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 首次启动延迟 2 分钟执行（错开服务启动峰值，与健康同步 1 分钟延迟错开）
	select {
	case <-time.After(2 * time.Minute):
		s.runAppleCalendarSync()
	case <-s.stopCh:
		return
	}

	for {
		select {
		case <-ticker.C:
			s.runAppleCalendarSync()
		case <-s.stopCh:
			return
		}
	}
}

// runAppleCalendarSync 执行一次 Apple 日历同步
func (s *Scheduler) runAppleCalendarSync() {
	s.recordRun("calendar_sync", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	s.runCalendarScriptSync(ctx)
}

// runCalendarScriptSync 通过 exec 调用日历同步脚本
func (s *Scheduler) runCalendarScriptSync(ctx context.Context) {
	scriptPath := s.calendarSyncCfg.ScriptPath
	if scriptPath == "" {
		scriptPath = "./homemate_calendar_sync.py"
	}
	pythonPath := s.calendarSyncCfg.PythonPath
	if pythonPath == "" {
		pythonPath = "/usr/bin/python3"
	}

	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		log.Printf("[SCHEDULER][CALENDAR] 脚本路径解析失败 (%s): %v", scriptPath, err)
		absScript = scriptPath
	}
	if _, err := os.Stat(absScript); err != nil {
		log.Printf("[SCHEDULER][CALENDAR] 同步脚本不存在: %s (%v)", absScript, err)
		return
	}

	absDB := s.dbPath
	if abs, err := filepath.Abs(s.dbPath); err == nil {
		absDB = abs
	}

	timeout := s.calendarSyncCfg.ScriptTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	lookback := s.calendarSyncCfg.LookbackDays
	if lookback <= 0 {
		lookback = 30
	}
	lookahead := s.calendarSyncCfg.LookaheadDays
	if lookahead <= 0 {
		lookahead = 30
	}

	// 构造命令：python3 <script> --lookback N --lookahead M
	cmd := exec.CommandContext(runCtx, pythonPath, absScript,
		"--lookback", fmt.Sprintf("%d", lookback),
		"--lookahead", fmt.Sprintf("%d", lookahead))
	cmd.Dir = filepath.Dir(absScript)
	// 剥离 PYTHONHOME/PYTHONPATH（对齐 env -u 用法），注入 HOMEMATE_DB 写同一数据库
	cmd.Env = s.buildCalendarScriptEnv(absDB)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("[SCHEDULER][CALENDAR] 调用同步脚本: %s %s --lookback %d --lookahead %d",
		pythonPath, absScript, lookback, lookahead)

	if err := cmd.Run(); err != nil {
		// 检测 TCC 自动化授权错误（osascript -10004 / 权限违例 / not authorized）
		combined := stdout.String() + "\n" + stderr.String()
		if isCalendarTCCError(combined) {
			log.Printf("[SCHEDULER][CALENDAR] TCC 自动化授权失效：osascript 无法访问 Calendar.app")
			log.Printf("[SCHEDULER][CALENDAR] 请在「系统设置 > 隐私与安全性 > 自动化」中为相应进程重新勾选 Calendar，本次不重试")
			return
		}
		log.Printf("[SCHEDULER][CALENDAR] 同步脚本执行失败: %v", err)
		if stderr.Len() > 0 {
			log.Printf("[SCHEDULER][CALENDAR] 脚本 stderr: %s", strings.TrimSpace(stderr.String()))
		}
		return
	}

	out := stdout.String()
	if len(out) > 0 {
		tail := out
		if len(tail) > 1024 {
			tail = "..." + tail[len(tail)-1024:]
		}
		log.Printf("[SCHEDULER][CALENDAR] 脚本输出:\n%s", strings.TrimSpace(tail))
	}

	// 解析 成功/跳过/失败 计数（脚本输出 "成功: N | 跳过: M | 失败: K"）
	ok := parseCalendarCount(out, "成功")
	skip := parseCalendarCount(out, "跳过")
	fail := parseCalendarCount(out, "失败")
	log.Printf("[SCHEDULER][CALENDAR] 同步完成: 成功=%d 跳过=%d 失败=%d", ok, skip, fail)
}

// buildCalendarScriptEnv 构造日历同步脚本环境变量
func (s *Scheduler) buildCalendarScriptEnv(absDB string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PYTHONHOME=") || strings.HasPrefix(e, "PYTHONPATH=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOMEMATE_DB="+absDB)
	return env
}

// isCalendarTCCError 检测 osascript TCC 自动化授权错误
func isCalendarTCCError(text string) bool {
	low := strings.ToLower(text)
	if strings.Contains(low, "-10004") || strings.Contains(low, "10004") {
		return true
	}
	if strings.Contains(low, "权限违例") || strings.Contains(low, "权限错误") {
		return true
	}
	if strings.Contains(low, "not authorized") || strings.Contains(low, "not authorised") {
		return true
	}
	if strings.Contains(low, "not allowed assistive") || strings.Contains(low, "appleevent") ||
		strings.Contains(low, "automation") && strings.Contains(low, "calendar") {
		return true
	}
	return false
}

// parseCalendarCount 从脚本输出解析标签后的数字（如 "成功: 3"）
func parseCalendarCount(out, label string) int {
	needle := label + ":"
	idx := strings.LastIndex(out, needle)
	if idx < 0 {
		needle = label + " :"
		idx = strings.LastIndex(out, needle)
	}
	if idx < 0 {
		return 0
	}
	rest := out[idx+len(needle):]
	n := 0
	for _, r := range rest {
		if r < '0' || r > '9' {
			if n > 0 {
				break
			}
			continue // 跳过数字前的空白/分隔符
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ============ AI 分析 ============

func (s *Scheduler) dailyAIAnalysisLoop(hour int) {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			if now.Hour() == hour && now.Minute() < 30 {
				// 检查今天是否已执行
				s.mu.Lock()
				last, ok := s.lastRuns["ai_analysis"]
				s.mu.Unlock()
				if ok && last.Format("2006-01-02") == now.Format("2006-01-02") {
					continue
				}
				s.runAIAnalysis()
			}
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runAIAnalysis() {
	s.recordRun("ai_analysis", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Println("[SCHEDULER] 开始 AI 健康档案分析...")

	// 获取所有未分析的档案文件
	files, err := s.db.GetHealthRecordFiles(ctx, 0, "", 100)
	if err != nil {
		log.Printf("[SCHEDULER] 获取档案文件失败: %v", err)
		return
	}

	unanalyzed := 0
	for _, f := range files {
		if f.AnalyzedAt == nil {
			unanalyzed++
		}
	}

	if unanalyzed == 0 {
		log.Println("[SCHEDULER] 没有待分析的档案文件")
		return
	}

	log.Printf("[SCHEDULER] 发现 %d 个待分析文件，开始批量分析...", unanalyzed)

	// TODO: 集成实际 AI 分析
	// 当前为框架占位，实际部署时需要：
	// 1. 提取文件内容（PDF OCR、图片识别）
	// 2. 调用 OpenAI API 分析
	// 3. 保存分析结果到数据库

	// 生成综合报告
	periodEnd := time.Now().Format("2006-01-02")
	periodStart := time.Now().AddDate(0, -1, 0).Format("2006-01-02")

	// 获取有分析的文件并生成报告
	analyzedFiles, _ := s.db.GetHealthRecordFiles(ctx, 0, "", 50)
	var fileSummaries []string
	for _, f := range analyzedFiles {
		if f.AnalyzedAt != nil && f.Summary != "" {
			fileSummaries = append(fileSummaries, fmt.Sprintf("- [%s] %s: %s", f.RecordDate, f.Title, f.Summary))
		}
	}

	if len(fileSummaries) > 0 {
		report := &model.HealthAnalysisReport{
			ReportDate:  time.Now().Format("2006-01-02"),
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			Summary:     fmt.Sprintf("系统定期分析报告，涵盖 %d 份已分析档案", len(fileSummaries)),
			Details:     "# 定期健康分析报告\n\n" + fmt.Sprintf("本报告基于 %d 份健康档案生成。\n\n## 已分析档案\n\n%s\n\n## 分析说明\n\nAI 深度分析功能需要配置 OpenAI API Key。请在 config.yaml 中设置 `openai.api_key`。", len(fileSummaries), joinStrings(fileSummaries, "\n")),
			Metrics:     fmt.Sprintf(`{"total_files":%d,"analyzed_files":%d,"unanalyzed_files":%d}`, len(files), len(files)-unanalyzed, unanalyzed),
			Source:      "ai",
		}

		if _, err := s.db.CreateAnalysisReport(ctx, report); err != nil {
			log.Printf("[SCHEDULER] 保存分析报告失败: %v", err)
		} else {
			log.Println("[SCHEDULER] 定期分析报告已生成")
		}
	}
}

// ============ 周末推荐生成 ============

func (s *Scheduler) weekendRecommendLoop(weekday time.Weekday, hour int) {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			if now.Weekday() == weekday && now.Hour() == hour && now.Minute() < 30 {
				s.mu.Lock()
				last, ok := s.lastRuns["weekend_recommend"]
				s.mu.Unlock()
				if ok && last.Format("2006-01-02") == now.Format("2006-01-02") {
					continue
				}
				s.runWeekendRecommend()
			}
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runWeekendRecommend() {
	s.recordRun("weekend_recommend", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 计算下一个周末日期
	now := time.Now()
	nextSaturday := now
	daysUntilSat := (6 - int(now.Weekday()) + 7) % 7
	if daysUntilSat == 0 && now.Hour() >= 12 {
		daysUntilSat = 7
	}
	if daysUntilSat > 0 {
		nextSaturday = now.AddDate(0, 0, daysUntilSat)
	}
	weekendDate := nextSaturday.Format("2006-01-02")
	weekendEnd := nextSaturday.AddDate(0, 0, 1).Format("2006-01-02")

	log.Printf("[SCHEDULER] 生成周末推荐 (%s ~ %s)...", weekendDate, weekendEnd)

	// 检查是否已有有效推荐
	existing, err := s.db.GetValidWeekendRecommendation(ctx, "all", weekendDate)
	if err == nil && existing != nil {
		log.Printf("[SCHEDULER] 周末推荐已存在且有效 (source=%s)", existing.Source)
		return
	}

	// TODO: 集成 AI 推荐生成
	// 当前生成离线默认推荐
	proposals := []map[string]interface{}{
		{
			"title":    "家庭公园野餐",
			"category": "outdoor",
			"duration": "半天",
			"cost":     "低",
			"tips":     "准备野餐垫、水果和三明治，选择有遮阴的草地",
		},
		{
			"title":    "家庭电影日",
			"category": "indoor",
			"duration": "2-3小时",
			"cost":     "低",
			"tips":     "提前选好电影，准备爆米花和饮料",
		},
		{
			"title":    "博物馆/展览参观",
			"category": "culture",
			"duration": "半天",
			"cost":     "中",
			"tips":     "提前在线预约门票，预留充足时间",
		},
		{
			"title":    "家庭烹饪日",
			"category": "home",
			"duration": "2-3小时",
			"cost":     "低",
			"tips":     "选择一个新菜谱，全家分工合作",
		},
	}

	proposalsJSON, _ := json.Marshal(proposals)
	weatherData := `{"source":"offline","condition":"未知","temp":"--"}`

	expiresAt := nextSaturday.AddDate(0, 0, 3) // 下周二过期

	rec := &model.WeekendRecommendation{
		GeneratedFor: "all",
		WeekendDate:  weekendDate,
		WeatherData:  weatherData,
		Proposals:    string(proposalsJSON),
		Source:       "offline",
		CreatedAt:    time.Now(),
		ExpiresAt:    expiresAt,
	}

	if _, err := s.db.SaveWeekendRecommendation(ctx, rec); err != nil {
		log.Printf("[SCHEDULER] 保存周末推荐失败: %v", err)
	} else {
		log.Printf("[SCHEDULER] 周末推荐已生成 (source=offline, proposals=%d)", len(proposals))
	}
}

// ============ 辅助函数 ============

func weekdayName(w time.Weekday) string {
	names := map[time.Weekday]string{
		time.Sunday:    "周日",
		time.Monday:    "周一",
		time.Tuesday:   "周二",
		time.Wednesday: "周三",
		time.Thursday:  "周四",
		time.Friday:    "周五",
		time.Saturday:  "周六",
	}
	return names[w]
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// Status 调度器状态信息
// v3.9.13: 遍历 taskRegistry 返回所有已注册任务（含未执行过的），
// 补充 next_run / last_status 字段（前端 index.html 已期望）。
func (s *Scheduler) Status() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]map[string]interface{}, 0, len(s.taskRegistry))
	for _, tm := range s.taskRegistry {
		lastRun, hasRun := s.lastRuns[tm.Task]
		status := s.lastStatus[tm.Task]
		entry := map[string]interface{}{
			"task":    tm.Task,
			"name":    tm.Name,
			"enabled": tm.Enabled,
		}
		if hasRun {
			entry["last_run"] = lastRun.Format(time.RFC3339)
			entry["last_run_ago"] = time.Since(lastRun).Truncate(time.Second).String()
			entry["last_status"] = status
			// next_run = last_run + interval（仅对固定间隔任务可预测）
			if tm.Interval > 0 {
				next := lastRun.Add(tm.Interval)
				entry["next_run"] = next.Format(time.RFC3339)
			}
		} else {
			entry["last_run"] = ""
			entry["last_run_ago"] = ""
			entry["last_status"] = ""
			entry["next_run"] = ""
		}
		tasks = append(tasks, entry)
	}

	return map[string]interface{}{
		"running": s.running,
		"tasks":   tasks,
	}
}

// ============ 历史数据清理 ============

// cleanupLoop 每日指定小时执行清理
func (s *Scheduler) cleanupLoop(hour int) {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			if now.Hour() == hour {
				s.runCleanup()
			}
		}
	}
}

// runCleanup 执行历史数据清理
func (s *Scheduler) runCleanup() {
	s.recordRun("cleanup", nil)
	log.Println("[CLEANUP] 开始清理历史数据（归档模式）...")

	if s.db == nil {
		log.Println("[CLEANUP] 数据库不可用，跳过")
		return
	}

	ctx := context.Background()
	now := time.Now()
	var totalArchived int64

	// TTL 映射：表 → 保留天数（归档后从活跃表删除）
	ttls := map[string]int{
		"news":           30,
		"notifications":  90,
		"points_records": 180,
		"chat_messages":  365,
		"message_board":  180,
		"device_data":    90,
	}
	// 表 → 日志名
	names := map[string]string{
		"news": "新闻", "notifications": "通知", "points_records": "积分记录",
		"chat_messages": "聊天消息", "message_board": "留言板", "device_data": "设备数据",
	}
	// 1. 时间驱动：归档超 TTL 的记录（搬移到 *_archive 后从原表删除）
	for _, spec := range store.ArchiveTableSpecs() {
		days, ok := ttls[spec.Table]
		if !ok {
			continue
		}
		before := now.AddDate(0, 0, -days)
		n, err := s.db.ArchiveAndDeleteBefore(ctx, spec.Table, before)
		if err != nil {
			log.Printf("[CLEANUP] %s 归档失败: %v", names[spec.Table], err)
			continue
		}
		log.Printf("[CLEANUP] %s 归档: %d 条", names[spec.Table], n)
		totalArchived += n
	}

	// 2. 容量驱动：活跃表超上限则归档最旧部分
	var capArchived int64
	for _, spec := range store.ArchiveTableSpecs() {
		n, err := s.db.EnforceArchiveCap(ctx, spec.Table, spec.Cap)
		if err != nil {
			log.Printf("[CLEANUP] %s 容量归档失败: %v", names[spec.Table], err)
			continue
		}
		if n > 0 {
			log.Printf("[CLEANUP] %s 容量归档: %d 条（上限 %d）", names[spec.Table], n, spec.Cap)
			capArchived += n
		}
	}

	log.Printf("[CLEANUP] 清理完成，时间归档 %d 条 + 容量归档 %d 条 = 共归档 %d 条历史记录", totalArchived, capArchived, totalArchived+capArchived)
}
