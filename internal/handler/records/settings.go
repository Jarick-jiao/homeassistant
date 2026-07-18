package records

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// hospitalsSettingKey family_settings 表中存储医院列表的 key
const hospitalsSettingKey = "hospitals"

// defaultHospitals 默认医院列表（首次加载或重置时使用）
var defaultHospitals = []string{}

// GetHospitalsHandler 返回就诊医院列表
// GET /api/settings/hospitals
func GetHospitalsHandler(c *gin.Context) {
	dbVal, exists := c.Get("db")
	if !exists || dbVal == nil {
		response.BadRequest(c, "数据库不可用")
		return
	}
	db, ok := dbVal.(*store.DB)
	if !ok {
		response.BadRequest(c, "数据库类型错误")
		return
	}

	raw := db.GetSetting(c.Request.Context(), hospitalsSettingKey)
	if raw == "" {
		// 首次访问返回默认空数组（前端显示空 textarea）
		response.Success(c, gin.H{"hospitals": defaultHospitals})
		return
	}

	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		// JSON 解析失败，回退到空列表
		response.Success(c, gin.H{"hospitals": defaultHospitals})
		return
	}
	response.Success(c, gin.H{"hospitals": list})
}

// UpdateHospitalsHandler 保存就诊医院列表（需 admin）
// PUT /api/settings/hospitals  body: {"hospitals": ["医院A", "医院B"]}
func UpdateHospitalsHandler(c *gin.Context) {
	dbVal, exists := c.Get("db")
	if !exists || dbVal == nil {
		response.BadRequest(c, "数据库不可用")
		return
	}
	db, ok := dbVal.(*store.DB)
	if !ok {
		response.BadRequest(c, "数据库类型错误")
		return
	}

	var req struct {
		Hospitals []string `json:"hospitals"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	// 去重 + 去空字符串
	seen := make(map[string]bool)
	cleaned := make([]string, 0, len(req.Hospitals))
	for _, h := range req.Hospitals {
		h = trimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		cleaned = append(cleaned, h)
	}

	data, err := json.Marshal(cleaned)
	if err != nil {
		response.BadRequest(c, "序列化失败: "+err.Error())
		return
	}

	if err := db.SetSetting(c.Request.Context(), hospitalsSettingKey, string(data)); err != nil {
		response.BadRequest(c, "保存失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"hospitals": cleaned, "count": len(cleaned)})
}

// trimSpace 去除字符串首尾空白（兼容老 Go 版本无 strings.TrimSpace 也可，这里直接用 strings.TrimSpace）
func trimSpace(s string) string {
	// 复用标准库
	return strings.TrimSpace(s)
}
