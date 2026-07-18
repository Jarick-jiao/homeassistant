package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ============================================================
// v3.3.1 历史数据归档
// 策略：清理前先 INSERT INTO x_archive，再 DELETE FROM x
// 双驱动：时间（TTL）+ 容量（活跃行数上限）
// ============================================================

// ArchiveTableSpec 归档表规格
type ArchiveTableSpec struct {
	Table       string // 原表名
	Archive     string // 归档表名
	TimeColumn  string // 时间列名
	TimeWhere   string // 时间条件（附加，如 "AND is_hot=0"）
	Cap         int64  // 容量上限（超过则归档最旧）
}

// archiveSpecs 6 张高增长表的归档规格
var archiveSpecs = []ArchiveTableSpec{
	{"news", "news_archive", "published_at", "AND is_hot=0", 2000},
	{"points_records", "points_records_archive", "created_at", "", 5000},
	{"chat_messages", "chat_messages_archive", "timestamp", "", 5000},
	{"device_data", "device_data_archive", "received_at", "", 5000},
	{"notifications", "notifications_archive", "created_at", "AND read_at IS NOT NULL", 5000},
	{"message_board", "message_board_archive", "created_at", "AND read_at IS NOT NULL", 3000},
}

// ArchiveTableSpecs 返回所有归档规格（供外部查阅）
func ArchiveTableSpecs() []ArchiveTableSpec { return archiveSpecs }

// archiveColumns 各表列清单（保持与归档表建表顺序一致，用于 INSERT）
var archiveColumns = map[string][]string{
	"news":             {"id", "category", "title", "summary", "content", "source", "source_url", "image_url", "published_at", "is_hot", "created_at"},
	"points_records":   {"id", "member_name", "pts_type", "type_label", "title", "points", "created_at"},
	"chat_messages":    {"id", "member_id", "content", "role", "timestamp", "session_id"},
	"device_data":      {"id", "member_id", "device_type", "data_json", "received_at"},
	"notifications":    {"id", "member_id", "type", "title", "body", "data_json", "read_at", "pushed_at", "created_at"},
	"message_board":    {"id", "from_member_id", "to_member_id", "content", "parent_id", "pinned", "read_at", "created_at"},
}

// ArchiveAndDeleteBefore 时间驱动归档：搬移指定时间前的记录到归档表，再从原表删除
// 对于 timestamp 列（chat_messages，Unix 秒），调用方传 before.Unix() 时需特殊处理
func (db *DB) ArchiveAndDeleteBefore(ctx context.Context, table string, before time.Time) (int64, error) {
	spec := findSpec(table)
	if spec == nil {
		return 0, fmt.Errorf("未知归档表: %s", table)
	}
	cols := archiveColumns[table]
	if cols == nil {
		return 0, fmt.Errorf("未配置归档列: %s", table)
	}
	colList := joinCols(cols)
	// chat_messages 的 timestamp 列是 INTEGER，需用 Unix 秒比较
	var arg interface{}
	if table == "chat_messages" {
		arg = before.Unix()
	} else {
		arg = before
	}
	timeCond := fmt.Sprintf("%s < ?", spec.TimeColumn)
	if spec.TimeWhere != "" {
		timeCond = "(" + timeCond + " " + spec.TimeWhere + ")"
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 1. 搬移到归档表
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s, archived_at) SELECT %s, ? FROM %s WHERE %s",
		spec.Archive, colList, colList, spec.Table, timeCond)
	if _, err := tx.ExecContext(ctx, insertSQL, time.Now(), arg); err != nil {
		return 0, fmt.Errorf("归档搬移失败 %s: %w", table, err)
	}
	// 2. 从原表删除
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s", spec.Table, timeCond)
	res, err := tx.ExecContext(ctx, deleteSQL, arg)
	if err != nil {
		return 0, fmt.Errorf("归档删除失败 %s: %w", table, err)
	}
	deleted, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// EnforceArchiveCap 容量驱动归档：若活跃行数超过 maxRows，归档最旧的超出部分
// 分批处理（每批 1000 条）避免长事务锁
func (db *DB) EnforceArchiveCap(ctx context.Context, table string, maxRows int64) (int64, error) {
	spec := findSpec(table)
	if spec == nil {
		return 0, fmt.Errorf("未知归档表: %s", table)
	}
	cols := archiveColumns[table]
	if cols == nil {
		return 0, fmt.Errorf("未配置归档列: %s", table)
	}
	colList := joinCols(cols)

	var count int64
	if err := db.conn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", spec.Table)).Scan(&count); err != nil {
		return 0, err
	}
	if count <= maxRows {
		return 0, nil
	}
	toArchive := count - maxRows
	var totalArchived int64
	const batchSize = 1000
	for toArchive > 0 {
		batch := batchSize
		if toArchive < batchSize {
			batch = int(toArchive)
		}
		archived, err := db.archiveOldestBatch(ctx, spec, colList, batch)
		if err != nil {
			return totalArchived, err
		}
		if archived == 0 {
			break
		}
		totalArchived += archived
		toArchive -= archived
	}
	return totalArchived, nil
}

// archiveOldestBatch 归档单批最旧记录
func (db *DB) archiveOldestBatch(ctx context.Context, spec *ArchiveTableSpec, colList string, limit int) (int64, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 选出最旧的 limit 条 id（按 id ASC，最旧优先）
	idQuery := fmt.Sprintf("SELECT id FROM %s ORDER BY id ASC LIMIT ?", spec.Table)
	rows, err := tx.QueryContext(ctx, idQuery, limit)
	if err != nil {
		return 0, err
	}
	var ids []interface{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return 0, nil
	}

	// 搬移这批 id 到归档表
	placeholders := ""
	args := []interface{}{}
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	insertSQL := fmt.Sprintf("INSERT INTO %s (%s, archived_at) SELECT %s, ? FROM %s WHERE id IN (%s)",
		spec.Archive, colList, colList, spec.Table, placeholders)
	insertArgs := append([]interface{}{time.Now()}, args...)
	if _, err := tx.ExecContext(ctx, insertSQL, insertArgs...); err != nil {
		return 0, fmt.Errorf("容量归档搬移失败 %s: %w", spec.Table, err)
	}
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", spec.Table, placeholders)
	res, err := tx.ExecContext(ctx, deleteSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("容量归档删除失败 %s: %w", spec.Table, err)
	}
	deleted, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// ListArchived 分页查询归档表
// 返回原始行（map[string]interface{}）以便 handler 统一序列化
func (db *DB) ListArchived(ctx context.Context, table string, limit, offset int) ([]map[string]interface{}, int64, error) {
	spec := findSpec(table)
	if spec == nil {
		return nil, 0, fmt.Errorf("未知归档表: %s", table)
	}
	// 总数
	var total int64
	if err := db.conn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", spec.Archive)).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.conn.QueryContext(ctx,
		fmt.Sprintf("SELECT * FROM %s ORDER BY archived_at DESC LIMIT ? OFFSET ?", spec.Archive), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, 0, err
	}
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, 0, err
		}
		row := make(map[string]interface{})
		for i, c := range cols {
			row[c] = values[i]
		}
		result = append(result, row)
	}
	return result, total, rows.Err()
}

// CountActive 返回原表活跃行数
func (db *DB) CountActive(ctx context.Context, table string) (int64, error) {
	spec := findSpec(table)
	if spec == nil {
		return 0, fmt.Errorf("未知归档表: %s", table)
	}
	var count int64
	err := db.conn.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", spec.Table)).Scan(&count)
	return count, err
}

// IsArchivableTable 白名单校验
func IsArchivableTable(table string) bool {
	return findSpec(table) != nil
}

func findSpec(table string) *ArchiveTableSpec {
	for i := range archiveSpecs {
		if archiveSpecs[i].Table == table {
			return &archiveSpecs[i]
		}
	}
	return nil
}

func joinCols(cols []string) string {
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ","
		}
		out += c
	}
	return out
}

// 确保编译期引用 sql 包（ListArchived 用到 rows.Columns）
var _ = sql.ErrNoRows
