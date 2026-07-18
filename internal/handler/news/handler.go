package news

import (
	"strconv"
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
