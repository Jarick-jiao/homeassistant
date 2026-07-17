package trip

import (
	"encoding/json"
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

// ListTripsHandler 列出出行计划
func ListTripsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []model.TripPlan{})
		return
	}
	plans, err := db.GetTripPlans(c.Request.Context())
	if err != nil {
		response.Success(c, []model.TripPlan{})
		return
	}
	response.Success(c, plans)
}

// CreateTripHandler 创建出行计划
func CreateTripHandler(c *gin.Context) {
	var req struct {
		Title       string  `json:"title" binding:"required"`
		Destination string  `json:"destination"`
		StartDate   string  `json:"start_date"`
		EndDate     string  `json:"end_date"`
		Status      string  `json:"status"`
		PlanJSON    string  `json:"plan_json"`
		Members     []int64 `json:"members"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	if req.Status == "" {
		req.Status = "planning"
	}
	userID, _ := c.Get("userID")
	uid, _ := userID.(int64)

	membersJSON, _ := json.Marshal(req.Members)
	t := &model.TripPlan{
		Title: req.Title, Destination: req.Destination,
		StartDate: req.StartDate, EndDate: req.EndDate,
		Status: req.Status, PlanJSON: req.PlanJSON,
		CreatedBy: uid, MembersJSON: string(membersJSON),
	}
	id, err := db.CreateTripPlan(c.Request.Context(), t)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "message": "出行计划创建成功"})
}