package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/service/garmin"
	"github.com/homemate/server/internal/service/weather"
	botservice "github.com/homemate/server/internal/service/wechat"
	"github.com/homemate/server/internal/store"
)

// Scheduler 定时任务调度器
type Scheduler struct {
	db           *store.DB
	garminClient garmin.Client
	weatherClient weather.Client
	pusher       botservice.Pusher
	stopCh       chan struct{}
	wg           sync.WaitGroup
	mu           sync.Mutex
	running      bool
	lastRuns     map[string]time.Time // 记录各任务最后执行时间
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
		HealthSyncInterval:     6 * time.Hour,
		AIAnalysisHour:         8,
		WeekendRecommendWeekday: time.Thursday,
		WeekendRecommendHour:  19,
	}
}

// New 创建调度器
// garminClient/weatherClient/pusher 可为 nil（未配置时跳过对应功能）
func New(db *store.DB, garminClient garmin.Client, weatherClient weather.Client, pusher botservice.Pusher) *Scheduler {
	if pusher == nil {
		pusher = botservice.NewPusher("")
	}
	return &Scheduler{
		db:            db,
		garminClient:  garminClient,
		weatherClient: weatherClient,
		pusher:        pusher,
		stopCh:        make(chan struct{}),
		lastRuns:      make(map[string]time.Time),
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

	// 健康数据同步
	s.wg.Add(1)
	go s.healthSyncLoop(cfg.HealthSyncInterval)

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
	s.mu.Lock()
	s.lastRuns["health_sync"] = time.Now()
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Println("[SCHEDULER] 开始同步健康数据...")

	// 1. 同步天气数据（用于周末出行面板）
	if s.weatherClient != nil {
		s.syncWeatherData(ctx)
	} else {
		log.Println("[SCHEDULER] 天气客户端未配置，跳过天气同步")
	}

	// 2. 获取所有配置了数据源的成员
	configs, err := s.db.GetDataSourceConfigs(ctx)
	if err != nil {
		log.Printf("[SCHEDULER] 获取数据源配置失败: %v", err)
		return
	}

	today := time.Now().Format("2006-01-02")
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

	if username == "" || password == "" {
		log.Printf("[SCHEDULER] Garmin 凭证不完整 (member=%d, user=%s)", memberID, username)
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
		MemberID:   memberID,
		Date:       date,
		Source:     "garmin",
		Steps:      health.Steps,
		HeartRate:  health.HeartRate,
		SleepHours: health.SleepHours,
		SleepScore: health.SleepScore,
		Stress:     health.Stress,
		SpO2:       health.SpO2,
		BodyBattery: health.BodyBattery,
		Calories:   health.Calories,
		SyncedAt:   time.Now().Format(time.RFC3339),
	}

	if err := s.db.UpsertHealthDataCache(ctx, cache); err != nil {
		log.Printf("[SCHEDULER] 保存 Garmin 数据失败 (member=%d): %v", memberID, err)
		return 0
	}

	log.Printf("[SCHEDULER] Garmin 数据同步完成 (member=%d, date=%s, steps=%d, hr=%d)",
		memberID, date, health.Steps, health.HeartRate)
	return 1
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
	s.mu.Lock()
	s.lastRuns["ai_analysis"] = time.Now()
	s.mu.Unlock()

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
	s.mu.Lock()
	s.lastRuns["weekend_recommend"] = time.Now()
	s.mu.Unlock()

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
func (s *Scheduler) Status() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]map[string]interface{}, 0, 3)
	for name, lastRun := range s.lastRuns {
		tasks = append(tasks, map[string]interface{}{
			"name":      name,
			"last_run":  lastRun.Format(time.RFC3339),
			"last_run_ago": time.Since(lastRun).Truncate(time.Second).String(),
		})
	}

	return map[string]interface{}{
		"running":     s.running,
		"tasks":       tasks,
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
	s.lastRuns["cleanup"] = time.Now()
	log.Println("[CLEANUP] 开始清理历史数据...")

	if s.db == nil {
		log.Println("[CLEANUP] 数据库不可用，跳过")
		return
	}

	ctx := context.Background()
	now := time.Now()
	var totalDeleted int64

	// 1. 新闻：30 天前的非热点
	if n, err := s.db.DeleteNewsBefore(ctx, now.AddDate(0, 0, -30)); err == nil {
		log.Printf("[CLEANUP] 新闻清理: %d 条", n)
		totalDeleted += n
	}

	// 2. 通知：90 天前已读
	if n, err := s.db.DeleteNotificationsBefore(ctx, now.AddDate(0, 0, -90)); err == nil {
		log.Printf("[CLEANUP] 通知清理: %d 条", n)
		totalDeleted += n
	}

	// 3. 积分记录：180 天前
	if n, err := s.db.DeletePointsRecordsBefore(ctx, now.AddDate(0, 0, -180)); err == nil {
		log.Printf("[CLEANUP] 积分记录清理: %d 条", n)
		totalDeleted += n
	}

	// 4. 聊天消息：365 天前
	if n, err := s.db.DeleteChatMessagesBefore(ctx, now.AddDate(0, 0, -365)); err == nil {
		log.Printf("[CLEANUP] 聊天消息清理: %d 条", n)
		totalDeleted += n
	}

	// 5. 留言板：180 天前已读
	if n, err := s.db.DeleteMessagesBefore(ctx, now.AddDate(0, 0, -180)); err == nil {
		log.Printf("[CLEANUP] 留言板清理: %d 条", n)
		totalDeleted += n
	}

	// 6. 设备数据：90 天前
	if n, err := s.db.DeleteDeviceDataBefore(ctx, now.AddDate(0, 0, -90)); err == nil {
		log.Printf("[CLEANUP] 设备数据清理: %d 条", n)
		totalDeleted += n
	}

	log.Printf("[CLEANUP] 清理完成，共删除 %d 条历史记录", totalDeleted)
}

