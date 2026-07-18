package news

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/homemate/server/internal/model"
	"github.com/homemate/server/internal/pkg/response"
	"github.com/homemate/server/internal/store"
)

// getDB 从上下文获取数据库
func getDB(c *gin.Context) *store.DB {
	if dbVal, exists := c.Get("db"); exists && dbVal != nil {
		if db, ok := dbVal.(*store.DB); ok {
			return db
		}
	}
	return nil
}

// parsePublishedAt 解析发布时间（支持 RFC3339 / YYYY-MM-DD HH:MM:SS / 空值）
func parsePublishedAt(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t
		}
	}
	return time.Now()
}

// ListNewsHandler 列出新闻（公开，分页 + 分类过滤）
func ListNewsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	category := c.Query("category")
	if category == "" {
		category = "all"
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	news, total, err := db.ListNews(c.Request.Context(), category, limit, offset)
	if err != nil {
		response.InternalServerError(c, "查询失败")
		return
	}
	response.Success(c, gin.H{
		"list":  news,
		"total": total,
		"limit": limit,
		"offset": offset,
	})
}

// CreateNewsHandler 外部写入新闻（需 API Token: news:write）
func CreateNewsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	var req model.NewsCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "参数错误: category 和 title 必填")
		return
	}
	if req.Source == "" {
		req.Source = "api"
	}
	n := &model.News{
		Category:    req.Category,
		Title:       req.Title,
		Summary:     req.Summary,
		Content:     req.Content,
		Source:      req.Source,
		SourceURL:   req.SourceURL,
		ImageURL:    req.ImageURL,
		PublishedAt: parsePublishedAt(req.PublishedAt),
		IsHot:       req.IsHot,
	}
	id, err := db.CreateNews(c.Request.Context(), n)
	if err != nil {
		response.InternalServerError(c, "创建失败")
		return
	}
	response.Success(c, gin.H{"id": id})
}

// BatchCreateNewsHandler 批量写入新闻（需 API Token: news:write，最多 50 条/次）
func BatchCreateNewsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	var reqs []model.NewsCreateRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		response.BadRequest(c, "参数错误: 应为数组")
		return
	}
	if len(reqs) > 50 {
		response.BadRequest(c, "单次最多 50 条")
		return
	}
	news := make([]model.News, 0, len(reqs))
	for _, r := range reqs {
		if r.Source == "" {
			r.Source = "api"
		}
		news = append(news, model.News{
			Category: r.Category, Title: r.Title, Summary: r.Summary, Content: r.Content,
			Source: r.Source, SourceURL: r.SourceURL, ImageURL: r.ImageURL,
			PublishedAt: parsePublishedAt(r.PublishedAt), IsHot: r.IsHot,
		})
	}
	count, err := db.BatchCreateNews(c.Request.Context(), news)
	if err != nil {
		response.InternalServerError(c, "批量创建失败")
		return
	}
	response.Success(c, gin.H{"created": count, "total": len(news)})
}

// DeleteNewsHandler 删除单条新闻（管理员）
func DeleteNewsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := db.DeleteNews(c.Request.Context(), id); err != nil {
		response.NotFound(c, "新闻不存在")
		return
	}
	response.Success(c, nil)
}

// DeleteAllNewsHandler 清空全部新闻（管理员）
func DeleteAllNewsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	count, err := db.DeleteAllNews(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "清空失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": count})
}

// SeedDemoNewsHandler 初始化 Demo 新闻（管理员）
func SeedDemoNewsHandler(c *gin.Context) {
	db := getDB(c)
	if db == nil {
		response.InternalServerError(c, "数据库不可用")
		return
	}
	count, err := db.SeedDemoNews(c.Request.Context())
	if err != nil {
		response.InternalServerError(c, "初始化失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"seeded": count, "message": fmt.Sprintf("已初始化 %d 条 Demo 新闻", count)})
}

// ImportCSVHandler CSV 批量导入新闻（管理员）
// CSV 格式: category,title,summary,content,source,source_url,image_url,published_at,is_hot
// is_hot: 0/1 或 true/false
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
	header, err := reader.Read()
	if err != nil {
		response.BadRequest(c, "CSV格式错误: 无法读取表头")
		return
	}

	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(strings.ToLower(h))] = i
	}

	titleIdx, hasTitle := colMap["title"]
	if !hasTitle {
		titleIdx, hasTitle = colMap["标题"]
	}
	if !hasTitle {
		response.BadRequest(c, "CSV必须包含 title（标题）列")
		return
	}

	imported, skipped := 0, 0
	var batch []model.News
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

		category := getCol("category", "分类")
		if category == "" {
			category = "tech"
		}
		isHotStr := strings.ToLower(getCol("is_hot", "isHot", "热门"))
		isHot := isHotStr == "1" || isHotStr == "true" || isHotStr == "yes" || isHotStr == "是"

		batch = append(batch, model.News{
			Category:    category,
			Title:       title,
			Summary:     getCol("summary", "摘要"),
			Content:     getCol("content", "正文"),
			Source:      strDefault(getCol("source", "来源"), "csv"),
			SourceURL:   getCol("source_url", "sourceurl", "链接"),
			ImageURL:    getCol("image_url", "imageurl", "图片"),
			PublishedAt: parsePublishedAt(getCol("published_at", "publishedat", "发布时间")),
			IsHot:       isHot,
		})
		if len(batch) >= 50 {
			n, _ := db.BatchCreateNews(c.Request.Context(), batch)
			imported += n
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		n, _ := db.BatchCreateNews(c.Request.Context(), batch)
		imported += n
	}

	log.Printf("[NEWS] CSV 导入完成: %d 成功, %d 跳过", imported, skipped)
	response.Success(c, gin.H{
		"imported": imported,
		"skipped":  skipped,
		"message":  fmt.Sprintf("成功导入 %d 条新闻，跳过 %d 条", imported, skipped),
	})
}

// GenerateCSVTemplateHandler 生成新闻 CSV 模板下载
func GenerateCSVTemplateHandler(c *gin.Context) {
	template := `category,title,summary,content,source,source_url,image_url,published_at,is_hot
tech,大模型成本下降,新一代量化技术让家用设备也能运行百亿参数模型,正文内容详情,csv,https://example.com/news1,,2026-07-18,1
health,每日步行8000步降低心血管风险,最新队列研究证实步数与全因死亡率关系,正文内容详情,csv,https://example.com/news2,,2026-07-18,0
`
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=news_template.csv")
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM 支持 Excel 中文
	c.Writer.Write([]byte(template))
}

// strDefault 空字符串返回默认值
func strDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
