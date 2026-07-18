package weekend

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
)

// ExternalAddProposalHandler 外部写入休闲活动方案（需 API Token: weekend:write）
// 逻辑与 AddProposalHandler 一致，但 created_by 标记为系统(0)，Tips 标注外部来源
func ExternalAddProposalHandler(c *gin.Context) {
	var req struct {
		Title       string   `json:"title" binding:"required"`
		Description string   `json:"description"`
		Icon        string   `json:"icon"`
		Category    string   `json:"category"`
		Tags        []string `json:"tags"`
		Duration    string   `json:"duration"`
		Cost        string   `json:"cost"`
		Difficulty  string   `json:"difficulty"`
		SuitableFor string   `json:"suitable_for"`
		WeatherReq  string   `json:"weather_req"`
		MemberID    int64    `json:"member_id"`
		MemberName  string   `json:"member_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if req.Icon == "" { req.Icon = "📋" }
	if req.Duration == "" { req.Duration = "半天" }
	if req.Cost == "" { req.Cost = "低" }
	if req.Difficulty == "" { req.Difficulty = "easy" }
	if req.Category == "" { req.Category = "other" }
	if req.SuitableFor == "" { req.SuitableFor = "全家" }
	if req.WeatherReq == "" { req.WeatherReq = "无限制" }
	if req.MemberName == "" { req.MemberName = "外部接入" }

	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}

	tagsJSON, _ := json.Marshal(req.Tags)
	sourceName := "外部接入"
	if name, exists := c.Get("api_token_name"); exists {
		if n, ok := name.(string); ok && n != "" {
			sourceName = n
		}
	}
	p := &model.WeekendProposalDB{
		Title: req.Title, Description: req.Description, Icon: req.Icon,
		Category: req.Category, TagsJSON: string(tagsJSON), Duration: req.Duration,
		Cost: req.Cost, Difficulty: req.Difficulty, SuitableFor: req.SuitableFor,
		WeatherReq: req.WeatherReq, Tips: "由 " + req.MemberName + " 推荐（" + sourceName + "）",
		CreatedBy: req.MemberID, // 0 表示系统
	}
	id, err := db.CreateWeekendProposal(c.Request.Context(), p)
	if err != nil {
		response.InternalServerError(c, "创建方案失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"id": id, "title": req.Title, "message": "方案已添加"})
}
