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
