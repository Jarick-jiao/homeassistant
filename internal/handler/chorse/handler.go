package chorse

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/memberctx"
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
// v4.0（范式 §2.2）：认领人身份取自 JWT → family_members，请求体只传 task_id；
// 任务名称/图标/积分以服务端任务库为准；同人同任务存在未结认领时幂等拒绝。
func ClaimChorseHandler(c *gin.Context) {
	var req struct {
		TaskID int64 `json:"task_id" binding:"required"`
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
	member, err := memberctx.CurrentMember(c)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}

	// 任务校验：必须存在、未删除、已启用
	task, err := db.GetChorseTaskByID(c.Request.Context(), req.TaskID)
	if err != nil {
		response.BadRequest(c, "家务任务不存在或未启用")
		return
	}
	if !task.Enabled {
		response.BadRequest(c, "该家务任务已停用，不能认领")
		return
	}

	// 幂等：同一成员对同一任务存在 pending/completed 认领时拒绝
	has, err := db.HasActiveClaim(c.Request.Context(), member.ID, req.TaskID)
	if err != nil {
		response.InternalServerError(c, "校验认领状态失败: "+err.Error())
		return
	}
	if has {
		response.Conflict(c, "你已认领该家务且尚未验收，不能重复认领")
		return
	}

	// 自动指派验收人：第一个非执行人的成人（或被委派管理员）；家庭仅一人时允许自验收
	verifierID, verifierName := int64(0), ""
	members, _ := db.GetMembers(c.Request.Context())
	for _, m := range members {
		if m.ID != member.ID && (m.Role == "adult" || m.IsAdmin) {
			verifierID = m.ID
			verifierName = m.Name
			break
		}
	}
	if verifierID == 0 {
		verifierID = member.ID
		verifierName = member.Name
	}

	deadline := time.Now().Add(24 * time.Hour)
	claim := &model.ChorseClaimDB{
		TaskID: task.ID, TaskName: task.Name, TaskIcon: task.Icon,
		MemberID: member.ID, MemberName: member.Name,
		Deadline: &deadline, Status: "pending", Points: task.Points,
		VerifierID: verifierID, VerifierName: verifierName,
	}
	id, err := db.CreateChorseClaim(c.Request.Context(), claim)
	if err != nil {
		response.InternalServerError(c, "认领失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"claim_id":      id,
		"task_name":     task.Name,
		"member_name":   member.Name,
		"verifier_id":   verifierID,
		"verifier_name": verifierName,
		"points":        task.Points,
		"deadline":      deadline.Format("2006-01-02 15:04:05"),
		"status":        "pending",
	})
}

// CompleteChorseHandler 标记完成（仅认领人本人，身份取自 JWT）
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
	member, err := memberctx.CurrentMember(c)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}

	if err := db.CompleteChorseClaim(c.Request.Context(), req.ClaimID, member.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(c, "认领记录不存在")
			return
		}
		response.Forbidden(c, "标记失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "已标记完成，等待确认"})
}

// ConfirmChorseHandler 确认验收（仅该单的验收人本人或系统管理员，身份取自 JWT）
func ConfirmChorseHandler(c *gin.Context) {
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

	// 定位待确认认领
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

	// 身份校验：系统管理员（admin 账号或被委派成员）可验收；否则必须是该单验收人本人
	isAdmin := memberctx.IsAdmin(c)
	confirmerName := memberctx.Username(c)
	if !isAdmin {
		member, err := memberctx.CurrentMember(c)
		if err != nil {
			response.Forbidden(c, err.Error())
			return
		}
		if target.VerifierID != member.ID {
			response.Forbidden(c, fmt.Sprintf("仅验收人 %s 或管理员可确认验收", target.VerifierName))
			return
		}
		confirmerName = member.Name
	}

	claim, err := db.ConfirmChorseClaim(c.Request.Context(), req.ClaimID, 0, confirmerName)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"message": fmt.Sprintf("%s 已确认完成，%s 获得 %d 积分", confirmerName, claim.MemberName, claim.Points),
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

// normalizeDifficulty 将前端难度（easy/medium/hard 或中文）统一为中文，
// 与 seed 数据及 points_records.type_label 统计口径保持一致。
// 否则新增任务 difficulty='easy'，认领确认时 type_label='easy'，
// GetPointsByMember 的 CASE WHEN type_label='简单' 匹配不到 → 三维积分统计错误。
func normalizeDifficulty(d string) string {
	switch d {
	case "easy", "简单":
		return "简单"
	case "medium", "中等":
		return "中等"
	case "hard", "困难":
		return "困难"
	default:
		if d == "" {
			return "简单"
		}
		return d
	}
}

// normalizeCategory 将前端分类（英文）统一为中文，与 seed 数据一致，
// 否则新增任务 category='cleaning'，与 seed 的"清洁"不一致，分类分组与编辑回填错乱。
func normalizeCategory(c string) string {
	switch c {
	case "cleaning", "清洁":
		return "清洁"
	case "cooking", "厨房":
		return "厨房"
	case "laundry", "洗衣":
		return "洗衣"
	case "organizing", "整理":
		return "整理"
	case "other", "其他":
		return "其他"
	default:
		if c == "" {
			return "其他"
		}
		return c // 园艺/宠物等已存在中文分类原样保留
	}
}

// CreateChorseTaskHandler 新增家务任务（v3.9.10: 前端持久化）
func CreateChorseTaskHandler(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Icon        string `json:"icon"`
		Category    string `json:"category"`
		Difficulty  string `json:"difficulty"`
		Points      int    `json:"points"`
		Duration    string `json:"duration"`
		Description string `json:"description"`
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
	task := &model.ChorseTaskDB{
		Name:        req.Name,
		Icon:        req.Icon,
		Category:    normalizeCategory(req.Category),
		Difficulty:  normalizeDifficulty(req.Difficulty),
		Points:      req.Points,
		Duration:    req.Duration,
		Description: req.Description,
		Enabled:     true, // 新增任务默认启用
	}
	id, err := db.CreateChorseTask(c.Request.Context(), task)
	if err != nil {
		response.InternalServerError(c, "创建失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id})
}

// UpdateChorseTaskHandler 更新家务任务（v3.9.10）
func UpdateChorseTaskHandler(c *gin.Context) {
	var req struct {
		ID          int64  `json:"id" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Icon        string `json:"icon"`
		Category    string `json:"category"`
		Difficulty  string `json:"difficulty"`
		Points      int    `json:"points"`
		Duration    string `json:"duration"`
		Description string `json:"description"`
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
	task := &model.ChorseTaskDB{
		ID:          req.ID,
		Name:        req.Name,
		Icon:        req.Icon,
		Category:    normalizeCategory(req.Category),
		Difficulty:  normalizeDifficulty(req.Difficulty),
		Points:      req.Points,
		Duration:    req.Duration,
		Description: req.Description,
	}
	if err := db.UpdateChorseTask(c.Request.Context(), task); err != nil {
		response.InternalServerError(c, "更新失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": req.ID})
}

// DeleteChorseTaskHandler 删除家务任务（软删除，v3.9.10）
func DeleteChorseTaskHandler(c *gin.Context) {
	var req struct {
		ID int64 `json:"id" binding:"required"`
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
	if err := db.DeleteChorseTask(c.Request.Context(), req.ID); err != nil {
		response.InternalServerError(c, "删除失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": req.ID})
}