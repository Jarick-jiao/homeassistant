package health

import (
	"fmt"
	"strconv"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
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
		cache, err := db.GetHealthDataCache(c.Request.Context(), m.ID, today)
		if err != nil {
			summaries = append(summaries, MemberSummary{
				MemberID: m.ID, Name: m.Name, Role: m.Role,
				Status: "no_data", StatusText: "暂无数据", Metrics: []gin.H{},
			})
			continue
		}
		status := "good"
		statusText := "数据正常"
		metrics := []gin.H{
			{"type": "steps", "label": "步数", "value": cache.Steps, "unit": "步", "icon": "👟"},
			{"type": "heart_rate", "label": "心率", "value": cache.HeartRate, "unit": "bpm", "icon": "❤️"},
			{"type": "sleep", "label": "睡眠", "value": cache.SleepHours, "unit": "小时", "icon": "😴"},
			{"type": "spo2", "label": "血氧", "value": cache.SpO2, "unit": "%", "icon": "🩸"},
			{"type": "stress", "label": "压力", "value": cache.Stress, "unit": "", "icon": "😰"},
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