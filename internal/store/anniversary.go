package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/homemate/server/internal/model"
)

// CreateAnniversary 创建纪念日
func (db *DB) CreateAnniversary(ctx context.Context, a *model.Anniversary) (int64, error) {
	if a.Type == "" {
		a.Type = "custom"
	}
	if a.NotifyDays <= 0 {
		a.NotifyDays = 1
	}
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO anniversaries (title, date, type, member_id, description, is_lunar, notify_days, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		a.Title, a.Date, a.Type, a.MemberID, a.Description, a.IsLunar, a.NotifyDays, a.CreatedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListAnniversaries 列出全部纪念日
func (db *DB) ListAnniversaries(ctx context.Context) ([]model.Anniversary, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, title, date, type, member_id, description, is_lunar, notify_days, created_by, created_at FROM anniversaries ORDER BY date ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Anniversary
	for rows.Next() {
		var a model.Anniversary
		if err := rows.Scan(&a.ID, &a.Title, &a.Date, &a.Type, &a.MemberID, &a.Description, &a.IsLunar, &a.NotifyDays, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetAnniversaryByID 获取单条纪念日
func (db *DB) GetAnniversaryByID(ctx context.Context, id int64) (*model.Anniversary, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, title, date, type, member_id, description, is_lunar, notify_days, created_by, created_at FROM anniversaries WHERE id=?", id)
	var a model.Anniversary
	err := row.Scan(&a.ID, &a.Title, &a.Date, &a.Type, &a.MemberID, &a.Description, &a.IsLunar, &a.NotifyDays, &a.CreatedBy, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// UpdateAnniversary 更新纪念日
func (db *DB) UpdateAnniversary(ctx context.Context, id int64, a *model.Anniversary) error {
	if a.Type == "" {
		a.Type = "custom"
	}
	if a.NotifyDays <= 0 {
		a.NotifyDays = 1
	}
	res, err := db.conn.ExecContext(ctx,
		`UPDATE anniversaries SET title=?, date=?, type=?, member_id=?, description=?, is_lunar=?, notify_days=? WHERE id=?`,
		a.Title, a.Date, a.Type, a.MemberID, a.Description, a.IsLunar, a.NotifyDays, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteAnniversary 删除纪念日
func (db *DB) DeleteAnniversary(ctx context.Context, id int64) error {
	res, err := db.conn.ExecContext(ctx, "DELETE FROM anniversaries WHERE id=?", id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetUpcomingAnniversaries 获取未来 N 天内的纪念日
// 计算逻辑：将纪念日的月-日与当前年份组合，得到今年的纪念日；若已过则用明年
func (db *DB) GetUpcomingAnniversaries(ctx context.Context, days int) ([]model.AnniversaryView, error) {
	all, err := db.ListAnniversaries(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var result []model.AnniversaryView
	for _, a := range all {
		// 解析原日期（仅取月日）
		annDate, err := time.ParseInLocation("2006-01-02", a.Date, time.Local)
		if err != nil {
			continue
		}
		// 今年的纪念日
		thisYear := time.Date(now.Year(), annDate.Month(), annDate.Day(), 0, 0, 0, 0, time.Local)
		nextDate := thisYear
		if thisYear.Before(now) {
			// 已过，用明年
			nextDate = time.Date(now.Year()+1, annDate.Month(), annDate.Day(), 0, 0, 0, 0, time.Local)
		}
		daysUntil := int(nextDate.Sub(now).Hours() / 24)
		// 仅返回未来 N 天内的
		if daysUntil <= days {
			result = append(result, model.AnniversaryView{
				Anniversary: a,
				DaysUntil:  daysUntil,
				NextDate:   nextDate.Format("2006-01-02"),
			})
		}
	}
	return result, nil
}
