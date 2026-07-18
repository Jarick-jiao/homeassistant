package store

import (
	"context"
	"fmt"
	"time"

	"github.com/homemate/server/internal/model"
)

// ============ 健康档案文件 ============

// CreateHealthRecordFile 上传健康档案文件记录
func (db *DB) CreateHealthRecordFile(ctx context.Context, f *model.HealthRecordFile) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO health_record_files (member_id, title, category, record_date, description, hospital, clinic, file_name, file_size, file_type, file_path, summary, analysis, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', ?)`,
		f.MemberID, f.Title, f.Category, f.RecordDate, f.Description, f.Hospital, f.Clinic, f.FileName, f.FileSize, f.FileType, f.FilePath, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetHealthRecordFiles 获取成员的健康档案文件列表
func (db *DB) GetHealthRecordFiles(ctx context.Context, memberID int64, category string, limit int) ([]model.HealthRecordFile, error) {
	if limit <= 0 {
		limit = 50
	}
	query := "SELECT id, member_id, title, category, record_date, description, hospital, clinic, file_name, file_size, file_type, file_path, thumb_path, summary, analysis, analyzed_at, created_at FROM health_record_files"
	var args []interface{}
	where := ""

	if memberID > 0 {
		where += " WHERE member_id = ?"
		args = append(args, memberID)
	}
	if category != "" {
		if where != "" {
			where += " AND category = ?"
		} else {
			where += " WHERE category = ?"
		}
		args = append(args, category)
	}
	query += where + " ORDER BY record_date DESC, created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.HealthRecordFile
	for rows.Next() {
		var f model.HealthRecordFile
		if err := rows.Scan(&f.ID, &f.MemberID, &f.Title, &f.Category, &f.RecordDate, &f.Description, &f.Hospital, &f.Clinic, &f.FileName, &f.FileSize, &f.FileType, &f.FilePath, &f.ThumbPath, &f.Summary, &f.Analysis, &f.AnalyzedAt, &f.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetHealthRecordFileByID 根据 ID 获取文件
func (db *DB) GetHealthRecordFileByID(ctx context.Context, id int64) (*model.HealthRecordFile, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, member_id, title, category, record_date, description, hospital, clinic, file_name, file_size, file_type, file_path, thumb_path, summary, analysis, analyzed_at, created_at FROM health_record_files WHERE id=?", id)
	var f model.HealthRecordFile
	err := row.Scan(&f.ID, &f.MemberID, &f.Title, &f.Category, &f.RecordDate, &f.Description, &f.Hospital, &f.Clinic, &f.FileName, &f.FileSize, &f.FileType, &f.FilePath, &f.ThumbPath, &f.Summary, &f.Analysis, &f.AnalyzedAt, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateHealthRecordFileAnalysis 更新 AI 分析结果
func (db *DB) UpdateHealthRecordFileAnalysis(ctx context.Context, id int64, summary, analysis string) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE health_record_files SET summary=?, analysis=?, analyzed_at=? WHERE id=?",
		summary, analysis, time.Now(), id)
	return err
}

// DeleteHealthRecordFile 删除文件记录
func (db *DB) DeleteHealthRecordFile(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM health_record_files WHERE id=?", id)
	return err
}

// ============ 健康分析报告 ============

// CreateAnalysisReport 创建 AI 分析报告
func (db *DB) CreateAnalysisReport(ctx context.Context, r *model.HealthAnalysisReport) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO health_analysis_reports (member_id, report_date, period_start, period_end, summary, details, metrics, source, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.MemberID, r.ReportDate, r.PeriodStart, r.PeriodEnd, r.Summary, r.Details, r.Metrics, r.Source, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetAnalysisReports 获取分析报告列表
func (db *DB) GetAnalysisReports(ctx context.Context, memberID int64, limit int) ([]model.HealthAnalysisReport, error) {
	if limit <= 0 {
		limit = 20
	}
	query := "SELECT id, member_id, report_date, period_start, period_end, summary, details, metrics, source, created_at FROM health_analysis_reports"
	var args []interface{}
	if memberID > 0 {
		query += " WHERE member_id = ?"
		args = append(args, memberID)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []model.HealthAnalysisReport
	for rows.Next() {
		var r model.HealthAnalysisReport
		if err := rows.Scan(&r.ID, &r.MemberID, &r.ReportDate, &r.PeriodStart, &r.PeriodEnd, &r.Summary, &r.Details, &r.Metrics, &r.Source, &r.CreatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// GetLatestAnalysisReport 获取最新报告
func (db *DB) GetLatestAnalysisReport(ctx context.Context, memberID int64) (*model.HealthAnalysisReport, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, member_id, report_date, period_start, period_end, summary, details, metrics, source, created_at FROM health_analysis_reports WHERE member_id=? ORDER BY created_at DESC LIMIT 1", memberID)
	var r model.HealthAnalysisReport
	err := row.Scan(&r.ID, &r.MemberID, &r.ReportDate, &r.PeriodStart, &r.PeriodEnd, &r.Summary, &r.Details, &r.Metrics, &r.Source, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ============ 周末推荐缓存 ============

// SaveWeekendRecommendation 保存周末推荐（AI 或离线）
func (db *DB) SaveWeekendRecommendation(ctx context.Context, rec *model.WeekendRecommendation) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		`INSERT INTO weekend_recommendations (generated_for, weekend_date, weather_data, proposals, source, created_at, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(generated_for, weekend_date) DO UPDATE SET weather_data=excluded.weather_data, proposals=excluded.proposals, source=excluded.source, created_at=excluded.created_at, expires_at=excluded.expires_at`,
		rec.GeneratedFor, rec.WeekendDate, rec.WeatherData, rec.Proposals, rec.Source, rec.CreatedAt, rec.ExpiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetWeekendRecommendation 获取周末推荐
func (db *DB) GetWeekendRecommendation(ctx context.Context, generatedFor, weekendDate string) (*model.WeekendRecommendation, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, generated_for, weekend_date, weather_data, proposals, source, created_at, expires_at FROM weekend_recommendations WHERE generated_for=? AND weekend_date=?",
		generatedFor, weekendDate)
	var r model.WeekendRecommendation
	err := row.Scan(&r.ID, &r.GeneratedFor, &r.WeekendDate, &r.WeatherData, &r.Proposals, &r.Source, &r.CreatedAt, &r.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetValidWeekendRecommendation 获取有效推荐（未过期）
func (db *DB) GetValidWeekendRecommendation(ctx context.Context, generatedFor, weekendDate string) (*model.WeekendRecommendation, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, generated_for, weekend_date, weather_data, proposals, source, created_at, expires_at FROM weekend_recommendations WHERE generated_for=? AND weekend_date=? AND expires_at > datetime('now')",
		generatedFor, weekendDate)
	var r model.WeekendRecommendation
	err := row.Scan(&r.ID, &r.GeneratedFor, &r.WeekendDate, &r.WeatherData, &r.Proposals, &r.Source, &r.CreatedAt, &r.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ============ 补充建表（在 migrate 中调用）============

// RecordExtraMigrations 档案相关额外迁移
func RecordExtraMigrations() []struct {
	Table  string
	Column string
	DDL    string
} {
	return []struct {
		Table  string
		Column string
		DDL    string
	}{
		{"health_record_files", "summary", "ALTER TABLE health_record_files ADD COLUMN summary TEXT DEFAULT ''"},
		{"health_record_files", "analysis", "ALTER TABLE health_record_files ADD COLUMN analysis TEXT DEFAULT ''"},
		{"health_record_files", "analyzed_at", "ALTER TABLE health_record_files ADD COLUMN analyzed_at DATETIME"},
		{"health_record_files", "description", "ALTER TABLE health_record_files ADD COLUMN description TEXT DEFAULT ''"},
		{"health_record_files", "hospital", "ALTER TABLE health_record_files ADD COLUMN hospital TEXT DEFAULT ''"},
		{"health_record_files", "clinic", "ALTER TABLE health_record_files ADD COLUMN clinic TEXT DEFAULT ''"},
	}
}

// CreateRecordTables 创建档案相关表
func (db *DB) createRecordTables() error {
	schema := `
CREATE TABLE IF NOT EXISTS health_record_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '其他',
    record_date TEXT DEFAULT '',
    description TEXT DEFAULT '',
    hospital TEXT DEFAULT '',
    clinic TEXT DEFAULT '',
    file_name TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    file_type TEXT DEFAULT '',
    file_path TEXT NOT NULL,
    thumb_path TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    analysis TEXT DEFAULT '',
    analyzed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS health_analysis_reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    report_date TEXT NOT NULL,
    period_start TEXT DEFAULT '',
    period_end TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    details TEXT DEFAULT '',
    metrics TEXT DEFAULT '{}',
    source TEXT DEFAULT 'ai',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS weekend_recommendations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    generated_for TEXT NOT NULL DEFAULT 'all',
    weekend_date TEXT NOT NULL,
    weather_data TEXT DEFAULT '{}',
    proposals TEXT DEFAULT '[]',
    source TEXT DEFAULT 'offline',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME,
    UNIQUE(generated_for, weekend_date)
);

CREATE INDEX IF NOT EXISTS idx_record_files_member ON health_record_files(member_id, category, record_date DESC);
CREATE INDEX IF NOT EXISTS idx_analysis_reports_member ON health_analysis_reports(member_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_weekend_recs_date ON weekend_recommendations(weekend_date, expires_at);
`
	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("创建档案表失败: %w", err)
	}
	return nil
}