package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/homemate/server/internal/model"
)

// CreateNews 创建新闻（按 source_url 去重 upsert）
func (db *DB) CreateNews(ctx context.Context, n *model.News) (int64, error) {
	// 若 source_url 存在则 upsert
	if n.SourceURL != "" {
		var existingID int64
		err := db.conn.QueryRowContext(ctx,
			"SELECT id FROM news WHERE source_url=?", n.SourceURL).Scan(&existingID)
		if err == nil {
			_, _ = db.conn.ExecContext(ctx,
				"UPDATE news SET title=?, summary=?, content=?, image_url=?, published_at=?, is_hot=?, category=? WHERE id=?",
				n.Title, n.Summary, n.Content, n.ImageURL, n.PublishedAt, n.IsHot, n.Category, existingID)
			return existingID, nil
		}
	}
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO news (category, title, summary, content, source, source_url, image_url, published_at, is_hot, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		n.Category, n.Title, n.Summary, n.Content, n.Source, n.SourceURL, n.ImageURL, n.PublishedAt, n.IsHot)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// BatchCreateNews 批量创建新闻
func (db *DB) BatchCreateNews(ctx context.Context, news []model.News) (int, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	for _, n := range news {
		// upsert by source_url
		if n.SourceURL != "" {
			var existingID int64
			err := tx.QueryRowContext(ctx, "SELECT id FROM news WHERE source_url=?", n.SourceURL).Scan(&existingID)
			if err == nil {
				_, _ = tx.ExecContext(ctx,
					"UPDATE news SET title=?, summary=?, content=?, image_url=?, published_at=?, is_hot=?, category=? WHERE id=?",
					n.Title, n.Summary, n.Content, n.ImageURL, n.PublishedAt, n.IsHot, n.Category, existingID)
				count++
				continue
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO news (category, title, summary, content, source, source_url, image_url, published_at, is_hot, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
			n.Category, n.Title, n.Summary, n.Content, n.Source, n.SourceURL, n.ImageURL, n.PublishedAt, n.IsHot)
		if err != nil {
			continue
		}
		count++
	}
	return count, tx.Commit()
}

// ListNews 列出新闻（分页 + 分类过滤）
func (db *DB) ListNews(ctx context.Context, category string, limit, offset int) ([]model.News, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	where := ""
	args := []interface{}{}
	if category != "" && category != "all" {
		where = "WHERE category=?"
		args = append(args, category)
	}
	// 查询总数
	var total int
	err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM news "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	// 查询列表
	query := "SELECT id, category, title, summary, content, source, source_url, image_url, published_at, is_hot, created_at FROM news " + where + " ORDER BY is_hot DESC, published_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var result []model.News
	for rows.Next() {
		var n model.News
		var content, sourceURL, imageURL sql.NullString
		if err := rows.Scan(&n.ID, &n.Category, &n.Title, &n.Summary, &content, &n.Source, &sourceURL, &imageURL, &n.PublishedAt, &n.IsHot, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		n.Content = content.String
		n.SourceURL = sourceURL.String
		n.ImageURL = imageURL.String
		result = append(result, n)
	}
	return result, total, rows.Err()
}

// DeleteNews 删除单条新闻
func (db *DB) DeleteNews(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, "DELETE FROM news WHERE id=?", id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteNewsBefore 删除指定时间前的非热点新闻
func (db *DB) DeleteNewsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"DELETE FROM news WHERE published_at < ? AND is_hot = 0", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteAllNews 清空全部新闻（管理员）
func (db *DB) DeleteAllNews(ctx context.Context) (int64, error) {
	res, err := db.conn.ExecContext(ctx, "DELETE FROM news")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SeedDemoNews 插入预设 Demo 新闻（按 source_url 去重，避免重复种子）
func (db *DB) SeedDemoNews(ctx context.Context) (int, error) {
	demos := []model.News{
		{Category: "tech", Title: "AI 大模型本地化部署成本下降 60%", Summary: "新一代量化技术让家用设备也能运行百亿参数模型", Source: "demo", SourceURL: "demo:news:ai-local", ImageURL: "", PublishedAt: time.Now(), IsHot: true},
		{Category: "tech", Title: "智能家居 Matter 2.0 协议正式发布", Summary: "统一标准覆盖更多设备类型，跨品牌互操作性提升", Source: "demo", SourceURL: "demo:news:matter2", ImageURL: "", PublishedAt: time.Now()},
		{Category: "health", Title: "每日步行 8000 步可显著降低心血管风险", Summary: "最新队列研究证实步数与全因死亡率的剂量反应关系", Source: "demo", SourceURL: "demo:news:walk8k", ImageURL: "", PublishedAt: time.Now(), IsHot: true},
		{Category: "health", Title: "青少年睡眠时长建议上调至 9 小时", Summary: "睡眠不足影响前额叶发育与情绪调节能力", Source: "demo", SourceURL: "demo:news:teen-sleep", ImageURL: "", PublishedAt: time.Now()},
		{Category: "edu", Title: "教育部新课标强化家庭实践类课程", Summary: "家校协同评价纳入劳动与生活技能模块", Source: "demo", SourceURL: "demo:news:edu-newstd", ImageURL: "", PublishedAt: time.Now()},
		{Category: "life", Title: "周末城市公园免费开放名单更新", Summary: "全国 200+ 城市级公园纳入免费开放计划", Source: "demo", SourceURL: "demo:news:park-free", ImageURL: "", PublishedAt: time.Now()},
		{Category: "finance", Title: "居民储蓄型保险利率年内第三次下调", Summary: "专家建议长期资金配置多元化应对利率下行", Source: "demo", SourceURL: "demo:news:insurance-rate", ImageURL: "", PublishedAt: time.Now()},
		{Category: "sports", Title: "城市马拉松季开启，家庭接力组别增设", Summary: "鼓励全家参与的运动赛事覆盖更多城市", Source: "demo", SourceURL: "demo:news:marathon-family", ImageURL: "", PublishedAt: time.Now()},
	}
	count, err := db.BatchCreateNews(ctx, demos)
	return count, err
}
