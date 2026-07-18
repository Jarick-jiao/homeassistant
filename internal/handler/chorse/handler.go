package chorse

import (
	"fmt"
	"log"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// ChorseTaskView 前端展示用的任务
type ChorseTaskView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
	Difficulty  string `json:"difficulty"`
	Points      int    `json:"points"`
	Duration    string `json:"duration"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// ChorseRecordView 前端展示用的完成记录
type ChorseRecordView struct {
	Member string `json:"member"`
	Task   string `json:"task"`
	Icon   string `json:"icon"`
	Points int    `json:"points"`
	Time   string `json:"time"`
}

// ChorseContribView 前端展示用的成员贡献
type ChorseContribView struct {
	Name   string `json:"name"`
	Done   int    `json:"done"`
	Points int    `json:"points"`
}

// ChorseDashboardResponse 家务面板响应
type ChorseDashboardResponse struct {
	Stats         StatsView          `json:"stats"`
	Tasks         []ChorseTaskView   `json:"tasks"`
	TodayRecords  []ChorseRecordView `json:"today_records"`
	MemberContrib []ChorseContribView `json:"member_contrib"`
}

// StatsView 统计概览
type StatsView struct {
	TotalPoints   int `json:"total_points"`
	Completed     int `json:"completed"`
	ActiveMembers int `json:"active_members"`
	Streak        int `json:"streak"`
}

func getDB(c *gin.Context) *store.DB {
	dbVal, _ := c.Get("db")
	if dbVal == nil {
		return nil
	}
	return dbVal.(*store.DB)
}

// GetChorseDashboardHandler 获取家务面板数据（公开路由）
func GetChorseDashboardHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, ChorseDashboardResponse{Tasks: []ChorseTaskView{}, TodayRecords: []ChorseRecordView{}, MemberContrib: []ChorseContribView{}, Stats: StatsView{}})
		return
	}

	// 查询任务列表
	tasks, err := db.ListChorseTasks(c.Request.Context())
	if err != nil {
		tasks = []model.ChorseTaskDB{}
	}
	taskViews := make([]ChorseTaskView, 0, len(tasks))
	for _, t := range tasks {
		taskViews = append(taskViews, ChorseTaskView{
			ID: t.ID, Name: t.Name, Icon: t.Icon, Category: t.Category,
			Difficulty: t.Difficulty, Points: t.Points, Duration: t.Duration, Description: t.Description,
			Enabled: t.Enabled,
		})
	}

	// 查询今日已完成
	completed, err := db.GetTodayCompletedClaims(c.Request.Context())
	if err != nil {
		completed = []model.ChorseClaimDB{}
	}

	totalPts := 0
	memberSet := map[string]bool{}
	records := make([]ChorseRecordView, 0, len(completed))
	contribMap := map[string]*ChorseContribView{}

	for _, cl := range completed {
		totalPts += cl.Points
		memberSet[cl.MemberName] = true
		records = append(records, ChorseRecordView{
			Member: cl.MemberName, Task: cl.TaskName, Icon: cl.TaskIcon,
			Points: cl.Points, Time: cl.ClaimedAt.Format("15:04"),
		})
		if _, ok := contribMap[cl.MemberName]; !ok {
			contribMap[cl.MemberName] = &ChorseContribView{Name: cl.MemberName}
		}
		contribMap[cl.MemberName].Done++
		contribMap[cl.MemberName].Points += cl.Points
	}

	contrib := make([]ChorseContribView, 0, len(contribMap))
	for _, v := range contribMap {
		contrib = append(contrib, *v)
	}

	response.Success(c, ChorseDashboardResponse{
		Stats:         StatsView{TotalPoints: totalPts, Completed: len(completed), ActiveMembers: len(memberSet)},
		Tasks:         taskViews,
		TodayRecords:  records,
		MemberContrib: contrib,
	})
}

// ClaimChorseHandler 认领家务
func ClaimChorseHandler(c *gin.Context) {
	var req struct {
		MemberID     int64  `json:"member_id" binding:"required"`
		MemberName   string `json:"member_name" binding:"required"`
		TaskID       int64  `json:"task_id" binding:"required"`
		TaskName     string `json:"task_name" binding:"required"`
		TaskIcon     string `json:"task_icon"`
		VerifierID   int64  `json:"verifier_id"`
		VerifierName string `json:"verifier_name"`
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

	points := 10
	// 查询任务获取实际积分
	tasks, _ := db.ListChorseTasks(c.Request.Context())
	for _, t := range tasks {
		if t.ID == req.TaskID {
			points = t.Points
			break
		}
	}

	// 自动指派验收人：若请求未指定，则选第一个非执行人的成人/admin
	verifierID := req.VerifierID
	verifierName := req.VerifierName
	if verifierID == 0 {
		members, _ := db.GetMembers(c.Request.Context())
		for _, m := range members {
			if m.ID != req.MemberID && (m.Role == "adult" || m.Role == "admin") {
				verifierID = m.ID
				verifierName = m.Name
				break
			}
		}
		// 如果家庭只有一个成员，允许自验收
		if verifierID == 0 {
			verifierID = req.MemberID
			verifierName = req.MemberName
		}
	}

	deadline := time.Now().Add(24 * time.Hour)
	claim := &model.ChorseClaimDB{
		TaskID: req.TaskID, TaskName: req.TaskName, TaskIcon: req.TaskIcon,
		MemberID: req.MemberID, MemberName: req.MemberName,
		Deadline: &deadline, Status: "pending", Points: points,
		VerifierID: verifierID, VerifierName: verifierName,
	}
	id, err := db.CreateChorseClaim(c.Request.Context(), claim)
	if err != nil {
		response.InternalServerError(c, "认领失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"claim_id":      id,
		"task_name":     req.TaskName,
		"member_name":   req.MemberName,
		"verifier_id":   verifierID,
		"verifier_name": verifierName,
		"points":        points,
		"deadline":       deadline.Format("2006-01-02 15:04:05"),
		"status":         "pending",
	})
}

// CompleteChorseHandler 标记完成
func CompleteChorseHandler(c *gin.Context) {
	var req struct {
		ClaimID int64 `json:"claim_id" binding:"required"`
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

	if err := db.CompleteChorseClaim(c.Request.Context(), req.ClaimID); err != nil {
		response.BadRequest(c, "标记失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "已标记完成，等待确认"})
}

// ConfirmChorseHandler 确认完成（仅验收人或 admin 可调用）
func ConfirmChorseHandler(c *gin.Context) {
	var req struct {
		ClaimID   int64  `json:"claim_id" binding:"required"`
		Confirmer string `json:"confirmer" binding:"required"`
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

	// 权限校验：查询 claim 的 verifier_name，仅验收人或 admin 可确认
	claims, _ := db.GetPendingChorseClaims(c.Request.Context())
	var target *model.ChorseClaimDB
	for i := range claims {
		if claims[i].ID == req.ClaimID {
			target = &claims[i]
			break
		}
	}
	if target == nil {
		response.BadRequest(c, "认领记录不存在或状态不允许确认")
		return
	}
	userRole, _ := c.Get("role")
	roleStr, _ := userRole.(model.Role)
	isAdminVal, _ := c.Get("isAdmin")
	isAdmin, _ := isAdminVal.(bool)
	if target.VerifierName != "" && target.VerifierName != req.Confirmer && roleStr != model.RoleAdmin && !isAdmin {
		response.BadRequest(c, fmt.Sprintf("仅验收人 %s 或管理员可确认验收", target.VerifierName))
		return
	}

	userID, _ := c.Get("userID")
	uid, _ := userID.(int64)
	claim, err := db.ConfirmChorseClaim(c.Request.Context(), req.ClaimID, uid, req.Confirmer)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"message": fmt.Sprintf("%s 已确认完成，%s 获得 %d 积分", req.Confirmer, claim.MemberName, claim.Points),
		"claim":   claim,
	})
}

// GetPendingClaimsHandler 获取待办认领列表
func GetPendingClaimsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, gin.H{"claims": []model.ChorseClaimDB{}})
		return
	}
	claims, err := db.GetPendingChorseClaims(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] 查询待办认领失败: %v", err)
		claims = []model.ChorseClaimDB{}
	}
	response.Success(c, gin.H{"claims": claims})
}

// GetTodayRecords 导出今日完成记录（供大屏调用）
func GetTodayRecords(c *gin.Context) []ChorseRecordView {
	db := getDB(c)
	if db == nil {
		return []ChorseRecordView{}
	}
	completed, err := db.GetTodayCompletedClaims(c.Request.Context())
	if err != nil {
		return []ChorseRecordView{}
	}
	records := make([]ChorseRecordView, 0, len(completed))
	for _, cl := range completed {
		records = append(records, ChorseRecordView{
			Member: cl.MemberName, Task: cl.TaskName, Icon: cl.TaskIcon,
			Points: cl.Points, Time: cl.ClaimedAt.Format("15:04"),
		})
	}
	return records
}

// GetMemberContrib 导出成员贡献（供大屏调用）
func GetMemberContrib(c *gin.Context) []ChorseContribView {
	records := GetTodayRecords(c)
	contribMap := map[string]*ChorseContribView{}
	for _, r := range records {
		if _, ok := contribMap[r.Member]; !ok {
			contribMap[r.Member] = &ChorseContribView{Name: r.Member}
		}
		contribMap[r.Member].Done++
		contribMap[r.Member].Points += r.Points
	}
	result := make([]ChorseContribView, 0, len(contribMap))
	for _, v := range contribMap {
		result = append(result, *v)
	}
	return result
}

// ListAllChorseTasksHandler 管理员查看所有任务（含禁用的）
func ListAllChorseTasksHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, []ChorseTaskView{})
		return
	}
	tasks, err := db.ListAllChorseTasks(c.Request.Context())
	if err != nil {
		response.Success(c, []ChorseTaskView{})
		return
	}
	views := make([]ChorseTaskView, 0, len(tasks))
	for _, t := range tasks {
		views = append(views, ChorseTaskView{
			ID: t.ID, Name: t.Name, Icon: t.Icon, Category: t.Category,
			Difficulty: t.Difficulty, Points: t.Points, Duration: t.Duration, Description: t.Description,
			Enabled: t.Enabled,
		})
	}
	response.Success(c, views)
}

// ToggleChorseTaskHandler 启用/禁用任务
func ToggleChorseTaskHandler(c *gin.Context) {
	var req struct {
		ID      int64 `json:"id" binding:"required"`
		Enabled bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	if err := db.ToggleChorseTask(c.Request.Context(), req.ID, req.Enabled); err != nil {
		response.BadRequest(c, "操作失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"enabled": req.Enabled})
}