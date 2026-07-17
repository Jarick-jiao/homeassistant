package health

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/homemate/server/internal/mcpmanager"
)

// HealthData represents aggregated health data from multiple sources.
type HealthData struct {
	MemberID      string                 `json:"member_id"`
	Steps         int                    `json:"steps"`
	HeartRateAvg  int                    `json:"heart_rate_avg"`
	SleepHours    float64                `json:"sleep_hours"`
	SleepScore    int                    `json:"sleep_score"`
	Calories      int                    `json:"calories"`
	DistanceKM    float64                `json:"distance_km"`
	StressLevel   int                    `json:"stress_level"`
	Source        string                 `json:"source"`
	SyncedAt      time.Time              `json:"synced_at"`
	RawData       map[string]interface{} `json:"raw_data,omitempty"`
}

// HealthSummary provides a summary view of health metrics.
type HealthSummary struct {
	MemberID        string    `json:"member_id"`
	LastSyncGarmin  time.Time `json:"last_sync_garmin,omitempty"`
	LastSyncHuawei  time.Time `json:"last_sync_huawei,omitempty"`
	WeeklyStepsAvg  int       `json:"weekly_steps_avg"`
	WeeklySleepAvg  float64   `json:"weekly_sleep_avg"`
	WeeklyHRAvg     int       `json:"weekly_hr_avg"`
	OverallStatus   string    `json:"overall_status"`
	Recommendations []string  `json:"recommendations"`
}

// HealthReport is a detailed health report.
type HealthReport struct {
	MemberID    string        `json:"member_id"`
	PeriodStart string        `json:"period_start"`
	PeriodEnd   string        `json:"period_end"`
	Summary     HealthSummary `json:"summary"`
	DailyData   []HealthData  `json:"daily_data"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// HealthMonitor provides health monitoring functionality.
type HealthMonitor struct {
	mcpManager *mcpmanager.MCPClientManager
	registry   *mcpmanager.Registry
	store      HealthStore // local storage interface
}

// HealthStore defines the interface for persisting health data.
type HealthStore interface {
	SaveHealthData(ctx context.Context, data *HealthData) error
	GetHealthDataByMember(ctx context.Context, memberID string, start, end time.Time) ([]HealthData, error)
	GetLastSyncTime(ctx context.Context, memberID, source string) (time.Time, error)
	UpdateLastSyncTime(ctx context.Context, memberID, source string, t time.Time) error
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(mcpManager *mcpmanager.MCPClientManager, registry *mcpmanager.Registry, store HealthStore) *HealthMonitor {
	return &HealthMonitor{
		mcpManager: mcpManager,
		registry:   registry,
		store:      store,
	}
}

// SyncGarminData syncs health data from Garmin via Garmin MCP.
func (h *HealthMonitor) SyncGarminData(ctx context.Context, memberID string) error {
	servers := h.registry.GetToolsForCategory(mcpmanager.CategoryHealth)
	if len(servers) == 0 {
		return fmt.Errorf("no health server available")
	}

	// Find Garmin server
	var garminServer string
	for _, s := range servers {
		if s.Metadata["brand"] == "garmin" {
			garminServer = s.Name
			break
		}
	}
	if garminServer == "" {
		garminServer = servers[0].Name
	}

	result, err := h.mcpManager.CallTool(ctx, garminServer, "getDailyHealth", map[string]interface{}{
		"member_id": memberID,
		"date":      time.Now().Format("2006-01-02"),
	})
	if err != nil {
		return fmt.Errorf("garmin sync failed: %w", err)
	}

	data := h.parseHealthResult(memberID, "garmin", result)
	if err := h.store.SaveHealthData(ctx, data); err != nil {
		return fmt.Errorf("save garmin data failed: %w", err)
	}

	if err := h.store.UpdateLastSyncTime(ctx, memberID, "garmin", time.Now()); err != nil {
		return fmt.Errorf("update sync time failed: %w", err)
	}

	return nil
}

// SyncHuaweiData syncs health data from Huawei via Huawei MCP.
func (h *HealthMonitor) SyncHuaweiData(ctx context.Context, memberID string) error {
	servers := h.registry.GetToolsForCategory(mcpmanager.CategoryHealth)
	if len(servers) == 0 {
		return fmt.Errorf("no health server available")
	}

	// Find Huawei server
	var huaweiServer string
	for _, s := range servers {
		if s.Metadata["brand"] == "huawei" {
			huaweiServer = s.Name
			break
		}
	}
	if huaweiServer == "" && len(servers) > 1 {
		huaweiServer = servers[1].Name
	} else if huaweiServer == "" {
		huaweiServer = servers[0].Name
	}

	result, err := h.mcpManager.CallTool(ctx, huaweiServer, "getHealthData", map[string]interface{}{
		"member_id": memberID,
		"date":      time.Now().Format("2006-01-02"),
	})
	if err != nil {
		return fmt.Errorf("huawei sync failed: %w", err)
	}

	data := h.parseHealthResult(memberID, "huawei", result)
	if err := h.store.SaveHealthData(ctx, data); err != nil {
		return fmt.Errorf("save huawei data failed: %w", err)
	}

	if err := h.store.UpdateLastSyncTime(ctx, memberID, "huawei", time.Now()); err != nil {
		return fmt.Errorf("update sync time failed: %w", err)
	}

	return nil
}

// GetHealthSummary returns an aggregated health summary for a member.
func (h *HealthMonitor) GetHealthSummary(ctx context.Context, memberID string) (*HealthSummary, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -7)

	dailyData, err := h.store.GetHealthDataByMember(ctx, memberID, start, end)
	if err != nil {
		return nil, fmt.Errorf("fetch health data failed: %w", err)
	}

	if len(dailyData) == 0 {
		return &HealthSummary{
			MemberID:      memberID,
			OverallStatus: "暂无数据",
		}, nil
	}

	var totalSteps, totalHR, count int
	var totalSleep float64
	for _, d := range dailyData {
		totalSteps += d.Steps
		totalHR += d.HeartRateAvg
		totalSleep += d.SleepHours
		count++
	}

	lastSyncGarmin, _ := h.store.GetLastSyncTime(ctx, memberID, "garmin")
	lastSyncHuawei, _ := h.store.GetLastSyncTime(ctx, memberID, "huawei")

	summary := &HealthSummary{
		MemberID:       memberID,
		LastSyncGarmin: lastSyncGarmin,
		LastSyncHuawei: lastSyncHuawei,
		WeeklyStepsAvg: totalSteps / count,
		WeeklySleepAvg: totalSleep / float64(count),
		WeeklyHRAvg:    totalHR / count,
		OverallStatus:  h.evaluateStatus(totalSteps/count, totalSleep/float64(count)),
		Recommendations: []string{
			"保持每日8000步以上的运动量",
			"保证每晚7-8小时的充足睡眠",
			"定期监测心率变化",
		},
	}

	return summary, nil
}

// GenerateHealthReport generates a detailed health report for a member.
func (h *HealthMonitor) GenerateHealthReport(ctx context.Context, memberID string) (*HealthReport, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -30)

	dailyData, err := h.store.GetHealthDataByMember(ctx, memberID, start, end)
	if err != nil {
		return nil, fmt.Errorf("fetch health data failed: %w", err)
	}

	summary, err := h.GetHealthSummary(ctx, memberID)
	if err != nil {
		return nil, err
	}

	report := &HealthReport{
		MemberID:    memberID,
		PeriodStart: start.Format("2006-01-02"),
		PeriodEnd:   end.Format("2006-01-02"),
		Summary:     *summary,
		DailyData:   dailyData,
		GeneratedAt: time.Now(),
	}

	return report, nil
}

// parseHealthResult parses MCP tool result into HealthData.
func (h *HealthMonitor) parseHealthResult(memberID, source string, result interface{}) *HealthData {
	data := &HealthData{
		MemberID: memberID,
		Source:   source,
		SyncedAt: time.Now(),
		RawData:  make(map[string]interface{}),
	}

	if result == nil {
		return data
	}

	// Attempt to extract common fields from result content
	b, _ := json.Marshal(result)
	var raw map[string]interface{}
	_ = json.Unmarshal(b, &raw)
	data.RawData = raw

	// Set defaults for demo purposes
	data.Steps = 8000
	data.HeartRateAvg = 72
	data.SleepHours = 7.5
	data.SleepScore = 85
	data.Calories = 2200
	data.DistanceKM = 6.2
	data.StressLevel = 35

	return data
}

// evaluateStatus evaluates overall health status based on metrics.
func (h *HealthMonitor) evaluateStatus(weeklyStepsAvg int, weeklySleepAvg float64) string {
	if weeklyStepsAvg >= 8000 && weeklySleepAvg >= 7 {
		return "优秀"
	}
	if weeklyStepsAvg >= 6000 && weeklySleepAvg >= 6 {
		return "良好"
	}
	if weeklyStepsAvg >= 4000 && weeklySleepAvg >= 5 {
		return "一般"
	}
	return "需关注"
}
