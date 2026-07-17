package weekend

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// WeekendDashboardResponse 周末出行面板
type WeekendDashboardResponse struct {
	Proposals     []ProposalView    `json:"proposals"`
	VoteResults   []VoteResultItem  `json:"vote_results"`
	WeekendDate   string            `json:"weekend_date"`
	Weather       WeekendWeather    `json:"weather"`
	SelectedPlan  *ProposalView     `json:"selected_plan,omitempty"`
}

// ProposalView 方案视图
type ProposalView struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Duration    string   `json:"duration"`
	Cost        string   `json:"cost"`
	Difficulty  string   `json:"difficulty"`
	SuitableFor string   `json:"suitable_for"`
	WeatherReq  string   `json:"weather_req"`
	Tips        string   `json:"tips"`
}

// VoteResultItem 投票结果统计
type VoteResultItem struct {
	ProposalID   int64    `json:"proposal_id"`
	ProposalName string   `json:"proposal_name"`
	VoteCount    int      `json:"vote_count"`
	Voters       []string `json:"voters"`
	Winning      bool     `json:"winning"`
}

// WeekendWeather 周末天气
type WeekendWeather struct {
	Saturday    string `json:"saturday"`
	Sunday      string `json:"sunday"`
	Temperature string `json:"temperature"`
	Advice      string `json:"advice"`
}

func getDB(c *gin.Context) *store.DB {
	dbVal, _ := c.Get("db")
	if dbVal == nil {
		return nil
	}
	return dbVal.(*store.DB)
}

func toProposalView(p model.WeekendProposalDB) ProposalView {
	var tags []string
	_ = json.Unmarshal([]byte(p.TagsJSON), &tags)
	if tags == nil {
		tags = []string{}
	}
	return ProposalView{
		ID: p.ID, Title: p.Title, Description: p.Description, Icon: p.Icon,
		Category: p.Category, Tags: tags, Duration: p.Duration, Cost: p.Cost,
		Difficulty: p.Difficulty, SuitableFor: p.SuitableFor, WeatherReq: p.WeatherReq, Tips: p.Tips,
	}
}

// GetWeekendDashboardHandler 获取周末出行面板数据
func GetWeekendDashboardHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.Success(c, WeekendDashboardResponse{Proposals: []ProposalView{}, VoteResults: []VoteResultItem{}})
		return
	}

	proposals, _ := db.ListWeekendProposals(c.Request.Context())
	views := make([]ProposalView, 0, len(proposals))
	for _, p := range proposals {
		views = append(views, toProposalView(p))
	}

	voteResults, _ := db.GetWeekendVoteResults(c.Request.Context())
	items := make([]VoteResultItem, 0, len(voteResults))
	for i, v := range voteResults {
		item := VoteResultItem{
			ProposalID: v.ProposalID, ProposalName: v.ProposalName,
			VoteCount: v.VoteCount, Voters: v.Voters,
		}
		if i == 0 && v.VoteCount > 0 {
			item.Winning = true
		}
		items = append(items, item)
	}

	response.Success(c, WeekendDashboardResponse{
		Proposals:   views,
		VoteResults: items,
		WeekendDate: getUpcomingWeekend(),
	})
}

// VoteProposalHandler 投票
func VoteProposalHandler(c *gin.Context) {
	var req struct {
		ProposalID int64  `json:"proposal_id" binding:"required"`
		MemberID   int64  `json:"member_id" binding:"required"`
		MemberName string `json:"member_name" binding:"required"`
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

	if err := db.AddWeekendVote(c.Request.Context(), req.ProposalID, req.MemberID, req.MemberName); err != nil {
		response.BadRequest(c, "投票失败（可能已投过或方案不存在）")
		return
	}

	results, _ := db.GetWeekendVoteResults(c.Request.Context())
	items := make([]VoteResultItem, 0, len(results))
	for i, v := range results {
		item := VoteResultItem{ProposalID: v.ProposalID, ProposalName: v.ProposalName, VoteCount: v.VoteCount, Voters: v.Voters}
		if i == 0 && v.VoteCount > 0 {
			item.Winning = true
		}
		items = append(items, item)
	}
	response.Success(c, gin.H{"message": "投票成功", "results": items})
}

// CancelVoteHandler 取消投票
func CancelVoteHandler(c *gin.Context) {
	var req struct {
		MemberID int64 `json:"member_id" binding:"required"`
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

	proposalID, err := db.RemoveWeekendVote(c.Request.Context(), req.MemberID)
	if err != nil {
		response.BadRequest(c, "未找到投票记录")
		return
	}

	results, _ := db.GetWeekendVoteResults(c.Request.Context())
	items := make([]VoteResultItem, 0, len(results))
	for i, v := range results {
		item := VoteResultItem{ProposalID: v.ProposalID, ProposalName: v.ProposalName, VoteCount: v.VoteCount, Voters: v.Voters}
		if i == 0 && v.VoteCount > 0 {
			item.Winning = true
		}
		items = append(items, item)
	}
	response.Success(c, gin.H{"message": "已取消投票", "proposal_id": proposalID, "results": items})
}

// ConfirmPlanHandler 确认选定方案
func ConfirmPlanHandler(c *gin.Context) {
	var req struct {
		ProposalID int64 `json:"proposal_id" binding:"required"`
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
	proposals, _ := db.ListWeekendProposals(c.Request.Context())
	var selected *ProposalView
	for _, p := range proposals {
		if p.ID == req.ProposalID {
			view := toProposalView(p)
			selected = &view
			break
		}
	}
	if selected == nil {
		response.BadRequest(c, "方案不存在")
		return
	}
	response.Success(c, gin.H{
		"message":       "出行计划已确认！",
		"selected_plan": selected,
		"weekend_date":  getUpcomingWeekend(),
	})
}

// AddProposalHandler 添加自定义方案
func AddProposalHandler(c *gin.Context) {
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

	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}

	tagsJSON, _ := json.Marshal(req.Tags)
	p := &model.WeekendProposalDB{
		Title: req.Title, Description: req.Description, Icon: req.Icon,
		Category: req.Category, TagsJSON: string(tagsJSON), Duration: req.Duration,
		Cost: req.Cost, Difficulty: req.Difficulty, SuitableFor: req.SuitableFor,
		WeatherReq: req.WeatherReq, Tips: "由 " + req.MemberName + " 推荐", CreatedBy: req.MemberID,
	}
	id, err := db.CreateWeekendProposal(c.Request.Context(), p)
	if err != nil {
		response.InternalServerError(c, "创建方案失败: "+err.Error())
		return
	}

	response.Success(c, gin.H{"id": id, "title": req.Title, "message": "方案已添加"})
}

func getUpcomingWeekend() string {
	now := time.Now()
	daysUntilSat := (6 - int(now.Weekday())) % 7
	if daysUntilSat == 0 && now.Weekday() != time.Saturday {
		daysUntilSat = 7
	}
	sat := now.AddDate(0, 0, daysUntilSat)
	sun := sat.AddDate(0, 0, 1)
	return sat.Format("2006-01-02") + " ~ " + sun.Format("2006-01-02")
}

// ImportCSVHandler CSV批量导入周末出行方案
// CSV格式: title,description,icon,category,tags,duration,cost,difficulty,suitable_for,weather_req
// tags 用分号分隔
func ImportCSVHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请上传CSV文件")
		return
	}
	f, err := file.Open()
	if err != nil {
		response.InternalServerError(c, "文件读取失败")
		return
	}
	defer f.Close()

	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}

	reader := csv.NewReader(f)
	reader.LazyQuotes = true
	header, err := reader.Read() // 跳过表头
	if err != nil {
		response.BadRequest(c, "CSV格式错误: 无法读取表头")
		return
	}

	// 映射表头列名到索引
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(strings.ToLower(h))] = i
	}

	// 必须列
	titleIdx, hasTitle := colMap["title"]
	if !hasTitle {
		titleIdx, hasTitle = colMap["标题"]
	}
	if !hasTitle {
		response.BadRequest(c, "CSV必须包含 title（标题）列")
		return
	}

	imported := 0
	skipped := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			skipped++
			continue
		}
		if len(record) <= titleIdx {
			skipped++
			continue
		}

		title := strings.TrimSpace(record[titleIdx])
		if title == "" {
			skipped++
			continue
		}

		getCol := func(keys ...string) string {
			for _, k := range keys {
				if idx, ok := colMap[k]; ok && idx < len(record) {
					return strings.TrimSpace(record[idx])
				}
			}
			return ""
		}

		description := getCol("description", "描述")
		icon := getCol("icon", "图标")
		category := getCol("category", "分类")
		tagsStr := getCol("tags", "标签")
		duration := getCol("duration", "时长")
		cost := getCol("cost", "费用")
		difficulty := getCol("difficulty", "难度")
		suitableFor := getCol("suitable_for", "suitablefor", "适合人群")
		weatherReq := getCol("weather_req", "weatherreq", "天气要求")

		if icon == "" {
			icon = "📋"
		}
		if category == "" {
			category = "other"
		}
		if duration == "" {
			duration = "半天"
		}
		if cost == "" {
			cost = "低"
		}
		if difficulty == "" {
			difficulty = "easy"
		}
		if suitableFor == "" {
			suitableFor = "全家"
		}
		if weatherReq == "" {
			weatherReq = "无限制"
		}

		var tags []string
		for _, t := range strings.Split(tagsStr, ";") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		tagsJSON, _ := json.Marshal(tags)

		p := &model.WeekendProposalDB{
			Title: title, Description: description, Icon: icon,
			Category: category, TagsJSON: string(tagsJSON), Duration: duration,
			Cost: cost, Difficulty: difficulty, SuitableFor: suitableFor,
			WeatherReq: weatherReq, Tips: "CSV 导入", CreatedBy: 0,
		}
		if _, err := db.CreateWeekendProposal(c.Request.Context(), p); err != nil {
			log.Printf("[WARN] CSV导入方案失败 [%s]: %v", title, err)
			skipped++
			continue
		}
		imported++
	}

	response.Success(c, gin.H{
		"imported": imported,
		"skipped":  skipped,
		"message":  fmt.Sprintf("成功导入 %d 个方案，跳过 %d 个", imported, skipped),
	})
}

// GenerateCSVTemplateHandler 生成CSV模板下载
func GenerateCSVTemplateHandler(c *gin.Context) {
	template := `title,description,icon,category,tags,duration,cost,difficulty,suitable_for,weather_req
杭州西湖一日游,游览西湖十景,🏞️,outdoor,自然;亲子;文化,全天,中等,medium,全家,晴天
科技馆探索,参观科学展览,🔬,indoor,科普;教育,半天,低,easy,亲子,无限制
露营野餐,湖边露营烧烤,⛺,outdoor,户外;美食;亲子,全天,高,medium,全家,晴天
图书馆阅读,安静阅读时光,📚,indoor,文化;安静,半天,免费,easy,全家,无限制
`
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=weekend_plans_template.csv")
	// 添加 BOM 以支持 Excel 中文
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	c.Writer.Write([]byte(template))
}