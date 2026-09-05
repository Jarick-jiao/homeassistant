package points

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// PointsRecordView 积分记录
type PointsRecordView struct {
	ID     int64  `json:"id"`
	Time   string `json:"time"`
	Member string `json:"member"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Points int    `json:"points"`
}

// MemberPointsView 成员积分概览
// v3.8.0 方案C：三类维度改为按家务难度划分（简单/进阶/挑战），每类都有真实数据
type MemberPointsView struct {
	Name      string `json:"name"`
	Total     int    `json:"total"`
	Easy      int    `json:"easy"`   // 简单任务积分
	Medium    int    `json:"medium"` // 进阶任务积分（中等难度）
	Hard      int    `json:"hard"`   // 挑战任务积分（困难难度）
	Level     string `json:"level"`
	NextLevel string `json:"next_level"`
	NextNeed  int    `json:"next_need"`
	Rank      int    `json:"rank"`
}

// PointsDashboardResponse 积分面板
type PointsDashboardResponse struct {
	FamilyTotal     int                `json:"family_total"`
	WeeklyGoal      int                `json:"weekly_goal"`
	WeeklyProgress  float64            `json:"weekly_progress"`
	Members         []MemberPointsView `json:"members"`
	Recent          []PointsRecordView `json:"recent"`
}

// 积分等级定义（MinTotal/NextThreshold 为绝对阈值）
var levelThresholds = []struct {
	MinTotal      int
	Level         string
	NextLevel     string
	NextThreshold int // 下一级的 MinTotal；末级填 0 表示满级
}{
	{0, "新手", "全能小能手", 500},
	{500, "全能小能手", "活力达人", 2500},
	{2500, "活力达人", "运动健将", 3000},
	{3000, "运动健将", "传奇管家", 4000},
	{4000, "传奇管家", "", 0},
}

func getLevel(total int) (level, nextLevel string, nextNeed int) {
	for i := len(levelThresholds) - 1; i >= 0; i-- {
		if total >= levelThresholds[i].MinTotal {
			lvl := levelThresholds[i]
			if lvl.NextThreshold == 0 {
				return lvl.Level, "已满级", 0
			}
			return lvl.Level, lvl.NextLevel, lvl.NextThreshold - total
		}
	}
	return "新手", "全能小能手", 500
}

func getDB(c *gin.Context) *store.DB {
	dbVal, _ := c.Get("db")
	if dbVal == nil {
		return nil
	}
	return dbVal.(*store.DB)
}

// GetPointsDashboardHandler 获取积分面板数据（公开路由）
func GetPointsDashboardHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, PointsDashboardResponse{Members: []MemberPointsView{}, Recent: []PointsRecordView{}, WeeklyGoal: 500})
		return
	}

	// 获取排行
	rankings, err := db.GetPointsRanking(c.Request.Context(), 20)
	if err != nil {
		rankings = []store.PointsRankingItem{}
	}

	members := make([]MemberPointsView, 0, len(rankings))
	familyTotal := 0
	for i, r := range rankings {
		easy, medium, hard, _, err := db.GetPointsByMember(c.Request.Context(), r.ID, r.Name)
		if err != nil {
			easy, medium, hard = 0, 0, 0
		}
		level, nextLevel, nextNeed := getLevel(r.Total)
		members = append(members, MemberPointsView{
			Name: r.Name, Total: r.Total, Easy: easy, Medium: medium, Hard: hard,
			Level: level, NextLevel: nextLevel, NextNeed: nextNeed, Rank: i + 1,
		})
		familyTotal += r.Total
	}

	// 获取最近记录
	recentRecords, err := db.GetRecentPointsRecords(c.Request.Context(), 50)
	if err != nil {
		recentRecords = []struct {
			ID     int64  `json:"id"`
			Time   string `json:"time"`
			Member string `json:"member"`
			Type   string `json:"type"`
			Title  string `json:"title"`
			Points int    `json:"points"`
		}{}
	}
	recent := make([]PointsRecordView, 0, len(recentRecords))
	for _, r := range recentRecords {
		recent = append(recent, PointsRecordView{
			ID: r.ID, Time: r.Time, Member: r.Member, Type: r.Type, Title: r.Title, Points: r.Points,
		})
	}

	// 读取周目标设置（默认 500）
	weeklyGoalStr := db.GetSetting(c.Request.Context(), "weekly_goal")
	weeklyGoal := 500
	if v, err := strconv.Atoi(weeklyGoalStr); err == nil && v > 0 {
		weeklyGoal = v
	}
	// 动态计算本周进度
	weeklySum, _ := db.GetWeeklyPointsSum(c.Request.Context())
	var weeklyProgress float64
	if weeklyGoal > 0 {
		weeklyProgress = float64(weeklySum) / float64(weeklyGoal)
		if weeklyProgress > 1.0 {
			weeklyProgress = 1.0
		}
	}

	response.Success(c, PointsDashboardResponse{
		FamilyTotal:    familyTotal,
		WeeklyGoal:     weeklyGoal,
		WeeklyProgress: weeklyProgress,
		Members:        members,
		Recent:         recent,
	})
}

// GetWeeklyGoalHandler 获取周目标
func GetWeeklyGoalHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, gin.H{"weekly_goal": 500})
		return
	}
	goalStr := db.GetSetting(c.Request.Context(), "weekly_goal")
	goal := 500
	if v, err := strconv.Atoi(goalStr); err == nil && v > 0 {
		goal = v
	}
	weeklySum, _ := db.GetWeeklyPointsSum(c.Request.Context())
	var progress float64
	if goal > 0 {
		progress = float64(weeklySum) / float64(goal)
		if progress > 1.0 {
			progress = 1.0
		}
	}
	response.Success(c, gin.H{
		"weekly_goal":     goal,
		"weekly_sum":     weeklySum,
		"weekly_progress": progress,
	})
}

// UpdateWeeklyGoalHandler 更新周目标（管理员）
func UpdateWeeklyGoalHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	var req struct {
		WeeklyGoal int `json:"weekly_goal"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	if req.WeeklyGoal <= 0 || req.WeeklyGoal > 100000 {
		response.BadRequest(c, "周目标应在 1~100000 之间")
		return
	}
	if err := db.SetSetting(c.Request.Context(), "weekly_goal", strconv.Itoa(req.WeeklyGoal)); err != nil {
		response.InternalServerError(c, "保存失败")
		return
	}
	response.Success(c, gin.H{"weekly_goal": req.WeeklyGoal})
}

// DeletePointsRecordHandler 删除积分记录（管理员）
func DeletePointsRecordHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的记录 ID")
		return
	}
	if err := db.DeletePointsRecord(c.Request.Context(), id); err != nil {
		response.InternalServerError(c, "删除失败")
		return
	}
	response.Success(c, nil)
}

// GetMembersSnapshot 导出成员积分快照（供大屏调用）
func GetMembersSnapshot(c *gin.Context) []MemberPointsView {
	db := getDB(c)
	if db == nil {
		return []MemberPointsView{}
	}
	rankings, err := db.GetPointsRanking(c.Request.Context(), 20)
	if err != nil {
		return []MemberPointsView{}
	}
	members := make([]MemberPointsView, 0, len(rankings))
	for i, r := range rankings {
		level, nextLevel, nextNeed := getLevel(r.Total)
		members = append(members, MemberPointsView{
			Name: r.Name, Total: r.Total, Level: level, NextLevel: nextLevel, NextNeed: nextNeed, Rank: i + 1,
		})
	}
	return members
}

// AddPoints 添加积分（供其他 handler 调用；v4.0 起必须带 memberID）
func AddPoints(c *gin.Context, memberID int64, memberName, ptsType, typeLabel, title string, points int) error {
	db := getDB(c)
	if db == nil {
		return nil
	}
	return db.AddPointsRecord(c.Request.Context(), memberID, memberName, ptsType, typeLabel, title, points)
}

// record 变更说明：原 points 包的 AddPoints 从内存存储操作改为数据库操作
// chorse 的 ConfirmChorseHandler 通过数据库事务直接写入积分
// 不再需要跨包的内存存储引用