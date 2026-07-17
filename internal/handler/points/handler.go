package points

import (
	"sort"
	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// PointsRecordView 积分记录
type PointsRecordView struct {
	Time   string `json:"time"`
	Member string `json:"member"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Points int    `json:"points"`
}

// MemberPointsView 成员积分概览
type MemberPointsView struct {
	Name      string `json:"name"`
	Total     int    `json:"total"`
	Sport     int    `json:"sport"`
	Health    int    `json:"health"`
	Labor     int    `json:"labor"`
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

// 积分等级定义
var levelThresholds = []struct {
	Total     int
	Level     string
	NextLevel string
	NextNeed  int
}{
	{0, "新手", "全能小能手", 500},
	{500, "全能小能手", "活力达人", 2000},
	{2500, "活力达人", "运动健将", 500},
	{3000, "运动健将", "传奇管家", 1000},
}

func getLevel(total int) (level, nextLevel string, nextNeed int) {
	for i := len(levelThresholds) - 1; i >= 0; i-- {
		if total >= levelThresholds[i].Total {
			return levelThresholds[i].Level, levelThresholds[i].NextLevel, levelThresholds[i].NextNeed - total
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
		rankings = []struct{ Name string `json:"name"`; Total int `json:"total"` }{}
	}

	members := make([]MemberPointsView, 0, len(rankings))
	familyTotal := 0
	for i, r := range rankings {
		sport, health, labor, _ := db.GetPointsByMember(c.Request.Context(), r.Name)
		level, nextLevel, nextNeed := getLevel(r.Total)
		members = append(members, MemberPointsView{
			Name: r.Name, Total: r.Total, Sport: sport, Health: health, Labor: labor,
			Level: level, NextLevel: nextLevel, NextNeed: nextNeed, Rank: i + 1,
		})
		familyTotal += r.Total
	}

	// 获取最近记录
	recentRecords, err := db.GetRecentPointsRecords(c.Request.Context(), 50)
	if err != nil {
		recentRecords = []struct{ Time string `json:"time"`; Member string `json:"member"`; Type string `json:"type"`; Title string `json:"title"`; Points int `json:"points"` }{}
	}
	recent := make([]PointsRecordView, 0, len(recentRecords))
	for _, r := range recentRecords {
		recent = append(recent, PointsRecordView{
			Time: r.Time, Member: r.Member, Type: r.Type, Title: r.Title, Points: r.Points,
		})
	}

	response.Success(c, PointsDashboardResponse{
		FamilyTotal:    familyTotal,
		WeeklyGoal:     500,
		WeeklyProgress: 0,
		Members:        members,
		Recent:         recent,
	})
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

// AddPoints 添加积分（供其他 handler 调用）
func AddPoints(c *gin.Context, memberName, ptsType, typeLabel, title string, points int) error {
	db := getDB(c)
	if db == nil {
		return nil
	}
	return db.AddPointsRecord(c.Request.Context(), memberName, ptsType, typeLabel, title, points)
}

// record 变更说明：原 points 包的 AddPoints 从内存存储操作改为数据库操作
// chorse 的 ConfirmChorseHandler 通过数据库事务直接写入积分
// 不再需要跨包的内存存储引用