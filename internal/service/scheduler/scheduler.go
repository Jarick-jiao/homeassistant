package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/store"
)

// Scheduler 定时任务调度器
type Scheduler struct {
	db        *store.DB
	stopCh    chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
	lastRuns  map[string]time.Time // 记录各任务最后执行时间
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
func New(db *store.DB) *Scheduler {
	return &Scheduler{
		db:       db,
		stopCh:   make(chan struct{}),
		lastRuns: make(map[string]time.Time),
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
	default:
		return fmt.Errorf("未知任务: %s", taskName)
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

	// 获取所有配置了数据源的成员
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

// syncGarminData 同步 Garmin 健康数据（框架实现）
func (s *Scheduler) syncGarminData(ctx context.Context, cfg model.DataSourceConfig, date string) int {
	// TODO: 集成 Garmin MCP Server 或 Garmin Connect API
	// 当前为框架实现，当 garmin-health MCP Server 连接后替换
	memberID := cfg.MemberID

	// 检查今天是否已同步
	existing, err := s.db.GetHealthDataCache(ctx, memberID, date)
	if err == nil && existing != nil && existing.Source == "garmin" {
		return 0 // 已同步
	}

	// 占位数据 - 实际部署时替换为 Garmin API 调用
	cache := &model.HealthDataCache{
		MemberID:  memberID,
		Date:      date,
		Source:    "garmin",
		SyncedAt:  time.Now().Format(time.RFC3339),
	}

	if err := s.db.UpsertHealthDataCache(ctx, cache); err != nil {
		log.Printf("[SCHEDULER] 保存 Garmin 数据失败 (member=%d): %v", memberID, err)
		return 0
	}

	log.Printf("[SCHEDULER] Garmin 数据同步完成 (member=%d, date=%s)", memberID, date)
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
		if f.AnalyzedAt.IsZero() {
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
		if !f.AnalyzedAt.IsZero() && f.Summary != "" {
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

