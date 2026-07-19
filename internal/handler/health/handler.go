package health

import (
	"fmt"
	"strconv"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/service/scheduler"
	"github.com/homemate/server/internal/store"
)

func getDB(c *gin.Context) *store.DB {
	dbVal, _ := c.Get("db")
	if dbVal == nil {
		return nil
	}
	return dbVal.(*store.DB)
}

// GetHealthSummaryHandler 获取所有家庭成员健康摘要
// v3.9.0: 按统一指标清单输出 20 个 Garmin 关注指标
// 每个指标包含: type(DB字段) + label(中文名) + key(Garmin API路径) + value + unit + icon + category
// 字段映射详见: docs/garmin-field-mapping.md
func GetHealthSummaryHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, gin.H{"members": []interface{}{}, "alerts": []string{}})
		return
	}
	members, err := db.GetMembers(c.Request.Context())
	if err != nil {
		response.Success(c, gin.H{"members": []interface{}{}, "alerts": []string{}})
		return
	}

	type MemberSummary struct {
		MemberID   int64  `json:"member_id"`
		Name       string `json:"name"`
		Role       string `json:"role"`
		Status     string `json:"status"`
		StatusText string `json:"status_text"`
		Metrics    []gin.H `json:"metrics"`
	}

	today := time.Now().Format("2006-01-02")
	summaries := make([]MemberSummary, 0, len(members))
	for _, m := range members {
		cache, cacheErr := db.GetHealthDataCache(c.Request.Context(), m.ID, today)
		customMetrics, _ := db.GetHealthMetricsByMember(c.Request.Context(), m.Name)

		// 两者都空才标 no_data
		if cacheErr != nil && len(customMetrics) == 0 {
			summaries = append(summaries, MemberSummary{
				MemberID: m.ID, Name: m.Name, Role: m.Role,
				Status: "no_data", StatusText: "暂无数据，请配置数据源", Metrics: []gin.H{},
			})
			continue
		}

		status := "good"
		statusText := "数据正常"
		metrics := []gin.H{}
		if cacheErr == nil {
			// v3.9.0: 统一指标清单（名称 + Garmin key + DB字段 + 单位 + 图标 + 分类）
			// 顺序: 活动 → 心率 → 睡眠 → 血氧 → 压力 → 身体电量 → 卡路里 → 呼吸 → HRV
			type metricDef struct {
				Type, Label, Key, Unit, Icon, Category string
				Value                                  interface{}
			}
			defs := []metricDef{
				// 活动
				{"steps", "步数", "totalSteps", "步", "👟", "活动", cache.Steps},
				{"total_distance_m", "总距离", "totalDistanceMeters", "米", "📏", "活动", cache.TotalDistanceM},
				{"daily_step_goal", "步数目标", "dailyStepGoal", "步", "🎯", "活动", cache.DailyStepGoal},
				// 心率
				{"heart_rate", "心率", "maxHeartRate", "bpm", "❤️", "心率", cache.HeartRate},
				{"min_heart_rate", "最低心率", "minHeartRate", "bpm", "💚", "心率", cache.MinHeartRate},
				{"resting_hr_7d_avg", "静息心率", "lastSevenDaysAvgRestingHeartRate", "bpm", "💓", "心率", cache.RestingHR7dAvg},
				// 睡眠
				{"sleep_hours", "睡眠时长", "sleepTimeSeconds÷3600", "小时", "😴", "睡眠", cache.SleepHours},
				{"sleep_score", "睡眠评分", "sleepScores.overall.value", "分", "💯", "睡眠", cache.SleepScore},
				{"deep_sleep_hours", "深睡时长", "deepSleepSeconds÷3600", "小时", "🌙", "睡眠", cache.DeepSleepHours},
				{"rem_sleep_hours", "REM睡眠", "remSleepSeconds÷3600", "小时", "🛌", "睡眠", cache.RemSleepHours},
				// 血氧
				{"spo2", "血氧", "latestSpo2", "%", "🫁", "血氧", cache.SpO2},
				// 压力
				{"stress", "平均压力", "averageStressLevel", "", "😰", "压力", cache.Stress},
				{"max_stress", "最大压力", "maxStressLevel", "", "😰", "压力", cache.MaxStress},
				{"stress_qualifier", "压力定性", "stressQualifier", "", "📊", "压力", cache.StressQualifier},
				// 身体电量
				{"body_battery", "身体电量", "bodyBatteryMostRecentValue", "", "🔋", "身体电量", cache.BodyBattery},
				{"body_battery_highest", "最高电量", "bodyBatteryHighestValue", "", "🔋", "身体电量", cache.BodyBatteryHighest},
				{"body_battery_lowest", "最低电量", "bodyBatteryLowestValue", "", "🔋", "身体电量", cache.BodyBatteryLowest},
				// 卡路里
				{"calories", "卡路里", "totalKilocalories", "kcal", "🔥", "卡路里", cache.Calories},
				// 呼吸
				{"avg_respiration", "平均呼吸", "avgWakingRespirationValue", "次/分", "🌬️", "呼吸", cache.AvgRespiration},
				// HRV
				{"avg_overnight_hrv", "夜间HRV", "avgOvernightHrv", "ms", "💜", "HRV", cache.AvgOvernightHRV},
			}
			for _, d := range defs {
				// 过滤零值/空值（未同步的字段不展示，避免一堆 0）
				if isMetricEmpty(d.Value) {
					continue
				}
				metrics = append(metrics, gin.H{
					"type":     d.Type,
					"label":    d.Label,
					"key":      d.Key,
					"value":    d.Value,
					"unit":     d.Unit,
					"icon":     d.Icon,
					"category": d.Category,
				})
			}
		} else {
			status = "partial"
			statusText = "仅自定义指标"
		}
		// 追加自定义指标
		for _, cm := range customMetrics {
			metrics = append(metrics, gin.H{
				"type": "custom_" + cm.Label, "label": cm.Label,
				"value": cm.Value, "unit": cm.Unit, "icon": cm.Icon,
				"status": cm.Status, "trend": cm.Trend,
				"category": "自定义",
			})
		}
		summaries = append(summaries, MemberSummary{
			MemberID: m.ID, Name: m.Name, Role: m.Role,
			Status: status, StatusText: statusText, Metrics: metrics,
		})
	}

	response.Success(c, gin.H{
		"members":      summaries,
		"last_updated": time.Now().Format("2006-01-02 15:04:05"),
		"alerts":       []string{},
	})
}

// isMetricEmpty 判断指标值是否为空（零值/空字符串）
func isMetricEmpty(v interface{}) bool {
	switch val := v.(type) {
	case int:
		return val == 0
	case int64:
		return val == 0
	case float64:
		return val == 0
	case string:
		return val == ""
	default:
		return v == nil
	}
}

// GetRealHealthDataHandler 获取真实健康数据
func GetRealHealthDataHandler(c *gin.Context) {
	memberIDStr := c.Query("member_id")
	date := c.Query("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	memberID := int64(1)
	if memberIDStr != "" {
		fmt.Sscanf(memberIDStr, "%d", &memberID)
	}
	db := getDB(c)
	if db == nil {
		response.Success(c, gin.H{"member_id": memberID, "date": date, "metrics": gin.H{}, "message": "数据库不可用"})
		return
	}
	cache, err := db.GetHealthDataCache(c.Request.Context(), memberID, date)
	if err != nil {
		response.Success(c, gin.H{"member_id": memberID, "date": date, "metrics": gin.H{}, "message": "暂无数据，请先配置数据源并同步"})
		return
	}
	response.Success(c, gin.H{"member_id": memberID, "date": date, "metrics": cache, "message": "数据已加载"})
}

// ListHealthRecordsHandler 列出健康记录
func ListHealthRecordsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []model.HealthRecord{})
		return
	}
	memberIDStr := c.Query("member_id")
	var memberID int64
	if memberIDStr != "" {
		fmt.Sscanf(memberIDStr, "%d", &memberID)
	}
	records, err := db.GetHealthRecords(c.Request.Context(), memberID, 100)
	if err != nil {
		records = []model.HealthRecord{}
	}
	response.Success(c, records)
}

// CreateHealthRecordHandler 创建健康记录
func CreateHealthRecordHandler(c *gin.Context) {
	var req model.HealthRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := db.CreateHealthRecord(c.Request.Context(), &req)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	req.ID = id
	response.Success(c, req)
}

// GetDataSourceConfigsHandler 获取数据源配置列表（API Key 脱敏）
func GetDataSourceConfigsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []model.DataSourceConfigView{})
		return
	}
	configs, err := db.GetDataSourceConfigs(c.Request.Context())
	if err != nil {
		response.Success(c, []model.DataSourceConfigView{})
		return
	}
	views := make([]model.DataSourceConfigView, 0, len(configs))
	for _, cfg := range configs {
		views = append(views, *cfg.ToView())
	}
	response.Success(c, views)
}

// SaveDataSourceConfigHandler 保存数据源配置
func SaveDataSourceConfigHandler(c *gin.Context) {
	var req model.DataSourceConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := db.SaveDataSourceConfig(c.Request.Context(), &req)
	if err != nil {
		response.InternalServerError(c, "保存失败: "+err.Error())
		return
	}
	req.ID = id
	response.Success(c, req)
}

// SyncHealthDataHandler 手动触发健康数据同步
func SyncHealthDataHandler(c *gin.Context) {
	schedVal, exists := c.Get("scheduler")
	if !exists || schedVal == nil {
		response.InternalServerError(c, "调度器未初始化")
		return
	}
	sched, ok := schedVal.(*scheduler.Scheduler)
	if !ok || sched == nil {
		response.InternalServerError(c, "调度器类型错误")
		return
	}
	if err := sched.TriggerManual("health_sync"); err != nil {
		response.BadRequest(c, "触发同步失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{
		"message":        "同步已触发，请稍后刷新查看",
		"metrics_synced": []string{"steps", "heart_rate", "sleep", "spo2", "body_battery", "stress"},
	})
}

// AddMetricHandler 添加自定义健康指标
func AddMetricHandler(c *gin.Context) {
	var req struct {
		MemberName string  `json:"member_name" binding:"required"`
		Label      string  `json:"label" binding:"required"`
		Value      float64 `json:"value" binding:"required"`
		Unit       string  `json:"unit"`
		Icon       string  `json:"icon"`
		Status     string  `json:"status"`
		Trend      string  `json:"trend"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	if req.Icon == "" { req.Icon = "📊" }
	if req.Status == "" { req.Status = "normal" }
	if req.Trend == "" { req.Trend = "stable" }
	m := &model.HealthMetric{
		MemberName: req.MemberName,
		Label:      req.Label,
		Value:      req.Value,
		Unit:       req.Unit,
		Icon:       req.Icon,
		Status:     req.Status,
		Trend:      req.Trend,
	}
	id, err := db.AddHealthMetric(c.Request.Context(), m)
	if err != nil {
		response.InternalServerError(c, "添加指标失败: "+err.Error())
		return
	}
	m.ID = id
	response.Success(c, m)
}

// ListMetricsHandler 获取所有健康指标
func ListMetricsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []model.HealthMetric{})
		return
	}
	metrics, err := db.GetAllHealthMetrics(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "查询失败: "+err.Error())
		return
	}
	if metrics == nil { metrics = []model.HealthMetric{} }
	response.Success(c, metrics)
}

// DeleteMetricHandler 删除健康指标
func DeleteMetricHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	if err := db.DeleteHealthMetric(c.Request.Context(), id); err != nil {
		response.BadRequest(c, "删除失败: "+err.Error())
		return
	}
	response.Success(c, nil)
}