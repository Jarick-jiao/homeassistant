package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/homemate/server/internal/config"
	"github.com/homemate/server/internal/model"
)

// DB SQLite 数据库访问层
type DB struct {
	conn *sql.DB
}

// InitDB 初始化数据库连接并创建表
func InitDB(cfg config.DatabaseConfig) (*DB, error) {
	dsn := cfg.Path
	if cfg.WALMode {
		dsn += "?_journal_mode=WAL&_busy_timeout=5000"
	}

	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 连接池配置
	conn.SetMaxOpenConns(cfg.MaxOpen)
	conn.SetMaxIdleConns(cfg.MaxIdle)
	conn.SetConnMaxLifetime(cfg.MaxLife)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	db := &DB{conn: conn}

	if err := db.createTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	if err := db.createRecordTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("创建档案表失败: %w", err)
	}

	if err := db.createHealthMetricTables(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("创建健康指标表失败: %w", err)
	}

	if err := db.seedChorseTasks(context.Background()); err != nil {
		log.Printf("[WARN] 预置家务任务失败: %v", err)
	}

	if err := db.seedAdminUser(context.Background()); err != nil {
		log.Printf("[WARN] 创建默认管理员失败: %v", err)
	}

	log.Println("[INFO] 数据库初始化完成:", cfg.Path)
	return db, nil
}

func (db *DB) createTables() error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'guest',
    name TEXT NOT NULL DEFAULT '',
    family_id INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS family_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'guest',
    age INTEGER,
    preferences_json TEXT,
    health_focus TEXT DEFAULT '',
    data_source_plugin TEXT DEFAULT 'manual',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS health_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    record_type TEXT NOT NULL,
    value TEXT,
    unit TEXT,
    recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    source TEXT,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS calendar_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER,
    title TEXT NOT NULL,
    description TEXT,
    start_time DATETIME,
    end_time DATETIME,
    date TEXT,
    time TEXT,
    location TEXT,
    event_type TEXT,
    type TEXT,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS trip_plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    destination TEXT,
    start_date TEXT,
    end_date TEXT,
    status TEXT DEFAULT 'planning',
    plan_json TEXT,
    created_by INTEGER,
    members_json TEXT,
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER,
    content TEXT NOT NULL,
    role TEXT NOT NULL,
    timestamp INTEGER NOT NULL DEFAULT (strftime('%s','now')),
    session_id TEXT,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER,
    title TEXT NOT NULL,
    content TEXT,
    remind_at DATETIME,
    status TEXT DEFAULT 'pending',
    channel TEXT DEFAULT 'app',
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS device_data (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER,
    device_type TEXT NOT NULL,
    data_json TEXT,
    received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS feedback_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER,
    item_name TEXT NOT NULL,
    item_type TEXT,
    rating INTEGER CHECK(rating >= 1 AND rating <= 5),
    comment TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS message_board (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_member_id INTEGER NOT NULL,
    to_member_id INTEGER,
    content TEXT NOT NULL,
    parent_id INTEGER,
    pinned INTEGER DEFAULT 0,
    read_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (from_member_id) REFERENCES family_members(id),
    FOREIGN KEY (to_member_id) REFERENCES family_members(id)
);

CREATE INDEX IF NOT EXISTS idx_message_board_to ON message_board(to_member_id, read_at);
CREATE INDEX IF NOT EXISTS idx_message_board_created ON message_board(created_at DESC);

CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    data_json TEXT,
    read_at DATETIME,
    pushed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE INDEX IF NOT EXISTS idx_notifications_member_read ON notifications(member_id, read_at);
CREATE INDEX IF NOT EXISTS idx_notifications_pushed ON notifications(pushed_at);

CREATE TABLE IF NOT EXISTS data_source_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    member_name TEXT,
    source_type TEXT,
    api_key TEXT,
    api_secret TEXT,
    user_id TEXT,
    is_active INTEGER DEFAULT 1,
    last_sync_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS health_data_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER NOT NULL,
    date TEXT NOT NULL,
    steps INTEGER DEFAULT 0,
    heart_rate INTEGER DEFAULT 0,
    sleep_hours REAL DEFAULT 0,
    sleep_score INTEGER DEFAULT 0,
    blood_pressure_sys INTEGER DEFAULT 0,
    blood_pressure_dia INTEGER DEFAULT 0,
    weight REAL DEFAULT 0,
    height REAL DEFAULT 0,
    stress INTEGER DEFAULT 0,
    spo2 INTEGER DEFAULT 0,
    body_battery INTEGER DEFAULT 0,
    calories INTEGER DEFAULT 0,
    source TEXT,
    synced_at TEXT,
    UNIQUE(member_id, date),
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS chorse_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    icon TEXT DEFAULT '',
    category TEXT DEFAULT 'other',
    difficulty TEXT DEFAULT 'easy',
    points INTEGER DEFAULT 10,
    duration TEXT DEFAULT '',
    description TEXT DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS chorse_claims (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    task_name TEXT NOT NULL,
    task_icon TEXT DEFAULT '',
    member_id INTEGER NOT NULL,
    member_name TEXT NOT NULL,
    claimed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deadline DATETIME,
    status TEXT DEFAULT 'pending',
    points INTEGER DEFAULT 10,
    verifier_id INTEGER DEFAULT 0,
    verifier_name TEXT DEFAULT '',
    confirmed_by TEXT,
    confirmed_at DATETIME,
    FOREIGN KEY (task_id) REFERENCES chorse_tasks(id),
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE TABLE IF NOT EXISTS points_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_name TEXT NOT NULL,
    pts_type TEXT NOT NULL,
    type_label TEXT NOT NULL,
    title TEXT NOT NULL,
    points INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS weekend_proposals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    icon TEXT DEFAULT '',
    category TEXT DEFAULT 'other',
    tags_json TEXT DEFAULT '[]',
    duration TEXT DEFAULT '半天',
    cost TEXT DEFAULT '低',
    difficulty TEXT DEFAULT 'easy',
    suitable_for TEXT DEFAULT '全家',
    weather_req TEXT DEFAULT '无限制',
    tips TEXT DEFAULT '',
    created_by INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS weekend_votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    proposal_id INTEGER NOT NULL,
    member_id INTEGER NOT NULL,
    member_name TEXT NOT NULL,
    voted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(proposal_id, member_id),
    FOREIGN KEY (proposal_id) REFERENCES weekend_proposals(id),
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

CREATE INDEX IF NOT EXISTS idx_health_records_member ON health_records(member_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_calendar_events_member ON calendar_events(member_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_health_cache_member_date ON health_data_cache(member_id, date);
CREATE INDEX IF NOT EXISTS idx_chorse_claims_status ON chorse_claims(status);
CREATE INDEX IF NOT EXISTS idx_points_records_member ON points_records(member_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_weekend_votes_proposal ON weekend_votes(proposal_id);

CREATE TABLE IF NOT EXISTS health_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    family_id INTEGER NOT NULL DEFAULT 1,
    member_name TEXT NOT NULL,
    label TEXT NOT NULL,
    value REAL NOT NULL,
    unit TEXT DEFAULT '',
    icon TEXT DEFAULT '📊',
    status TEXT DEFAULT 'normal',
    trend TEXT DEFAULT 'stable',
    recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS family_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("执行建表语句失败: %w", err)
	}
	return nil
}

// migrate 数据库迁移：为旧表添加新字段
func (db *DB) migrate() error {
	migrations := []struct {
		table  string
		column string
		ddl    string
	}{
		{"family_members", "health_focus", "ALTER TABLE family_members ADD COLUMN health_focus TEXT DEFAULT ''"},
		{"family_members", "data_source_plugin", "ALTER TABLE family_members ADD COLUMN data_source_plugin TEXT DEFAULT 'manual'"},
		{"calendar_events", "date", "ALTER TABLE calendar_events ADD COLUMN date TEXT"},
		{"calendar_events", "time", "ALTER TABLE calendar_events ADD COLUMN time TEXT"},
		{"calendar_events", "type", "ALTER TABLE calendar_events ADD COLUMN type TEXT"},
		{"trip_plans", "members_json", "ALTER TABLE trip_plans ADD COLUMN members_json TEXT"},
		{"chorse_tasks", "enabled", "ALTER TABLE chorse_tasks ADD COLUMN enabled INTEGER NOT NULL DEFAULT 0"},
		{"chorse_claims", "verifier_id", "ALTER TABLE chorse_claims ADD COLUMN verifier_id INTEGER DEFAULT 0"},
		{"chorse_claims", "verifier_name", "ALTER TABLE chorse_claims ADD COLUMN verifier_name TEXT DEFAULT ''"},
		{"family_members", "wechat_openid", "ALTER TABLE family_members ADD COLUMN wechat_openid TEXT DEFAULT ''"},
		{"calendar_events", "is_important", "ALTER TABLE calendar_events ADD COLUMN is_important INTEGER DEFAULT 0"},
		{"calendar_events", "recurrence_rule", "ALTER TABLE calendar_events ADD COLUMN recurrence_rule TEXT"},
		{"calendar_events", "reminder_minutes", "ALTER TABLE calendar_events ADD COLUMN reminder_minutes INTEGER DEFAULT 30"},
		{"calendar_events", "last_reminded_at", "ALTER TABLE calendar_events ADD COLUMN last_reminded_at DATETIME"},
		{"calendar_events", "created_by", "ALTER TABLE calendar_events ADD COLUMN created_by INTEGER"},
		{"calendar_events", "color", "ALTER TABLE calendar_events ADD COLUMN color TEXT DEFAULT ''"},
		{"health_record_files", "description", "ALTER TABLE health_record_files ADD COLUMN description TEXT DEFAULT ''"},
		{"health_record_files", "hospital", "ALTER TABLE health_record_files ADD COLUMN hospital TEXT DEFAULT ''"},
		{"health_record_files", "clinic", "ALTER TABLE health_record_files ADD COLUMN clinic TEXT DEFAULT ''"},
	}
	for _, m := range migrations {
		var count int
		err := db.conn.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?",
			m.table, m.column,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("检查列 %s.%s 失败: %w", m.table, m.column, err)
		}
		if count == 0 {
			if _, err := db.conn.Exec(m.ddl); err != nil {
				return fmt.Errorf("添加列 %s.%s 失败: %w", m.table, m.column, err)
			}
		}
	}
	return nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.conn.Close()
}

// GetConn 获取底层 *sql.DB（供事务使用）
func (db *DB) GetConn() *sql.DB {
	return db.conn
}

// ============ Users ============

// CreateUser 创建用户
func (db *DB) CreateUser(ctx context.Context, user *model.User) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO users (username, password_hash, role, name, family_id, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		user.Username, user.PasswordHash, user.Role, user.Name, user.FamilyID, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetUserByUsername 根据用户名获取用户
func (db *DB) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, username, password_hash, role, name, family_id, created_at FROM users WHERE username = ?", username)
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Name, &u.FamilyID, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID 根据 ID 获取用户
func (db *DB) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, username, password_hash, role, name, family_id, created_at FROM users WHERE id = ?", id)
	var u model.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Name, &u.FamilyID, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ============ Family Members ============

// CreateMember 创建家庭成员
func (db *DB) CreateMember(ctx context.Context, m *model.FamilyMember) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO family_members (user_id, name, role, age, preferences_json, health_focus, data_source_plugin, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		m.UserID, m.Name, m.Role, m.Age, m.PreferencesJSON, m.HealthFocus, m.DataSourcePlugin, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateMember 更新家庭成员
func (db *DB) UpdateMember(ctx context.Context, m *model.FamilyMember) error {
	_, err := db.conn.ExecContext(ctx,
		"UPDATE family_members SET name=?, role=?, age=?, preferences_json=?, health_focus=?, data_source_plugin=? WHERE id=?",
		m.Name, m.Role, m.Age, m.PreferencesJSON, m.HealthFocus, m.DataSourcePlugin, m.ID)
	return err
}

// UpdateMemberRole 仅更新成员角色（管理员委派用）
func (db *DB) UpdateMemberRole(ctx context.Context, id int64, role string) error {
	res, err := db.conn.ExecContext(ctx,
		"UPDATE family_members SET role=? WHERE id=?", role, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("成员不存在")
	}
	return nil
}

// CountAdmins 统计当前管理员数量（防止零管理员锁死）
func (db *DB) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := db.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM family_members WHERE role='admin'").Scan(&count)
	return count, err
}

// GetMemberByID 根据ID获取家庭成员
func (db *DB) GetMemberByID(ctx context.Context, id int64) (*model.FamilyMember, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, user_id, name, role, age, preferences_json, health_focus, data_source_plugin, created_at FROM family_members WHERE id=?", id)
	var m model.FamilyMember
	err := row.Scan(&m.ID, &m.UserID, &m.Name, &m.Role, &m.Age, &m.PreferencesJSON, &m.HealthFocus, &m.DataSourcePlugin, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetMembers 获取所有家庭成员
func (db *DB) GetMembers(ctx context.Context) ([]model.FamilyMember, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, user_id, name, role, age, preferences_json, health_focus, data_source_plugin, created_at FROM family_members ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]model.FamilyMember, 0)
	for rows.Next() {
		var m model.FamilyMember
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.Role, &m.Age, &m.PreferencesJSON, &m.HealthFocus, &m.DataSourcePlugin, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// DeleteMember 删除家庭成员
func (db *DB) DeleteMember(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM family_members WHERE id=?", id)
	return err
}

// ============ Health Records ============

// CreateHealthRecord 创建健康记录
func (db *DB) CreateHealthRecord(ctx context.Context, r *model.HealthRecord) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO health_records (member_id, record_type, value, unit, recorded_at, source) VALUES (?, ?, ?, ?, ?, ?)",
		r.MemberID, r.RecordType, r.Value, r.Unit, r.RecordedAt, r.Source)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetHealthRecords 获取家庭成员的健康记录
func (db *DB) GetHealthRecords(ctx context.Context, memberID int64, limit int) ([]model.HealthRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, member_id, record_type, value, unit, recorded_at, source FROM health_records WHERE member_id = ? ORDER BY recorded_at DESC LIMIT ?",
		memberID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []model.HealthRecord
	for rows.Next() {
		var r model.HealthRecord
		if err := rows.Scan(&r.ID, &r.MemberID, &r.RecordType, &r.Value, &r.Unit, &r.RecordedAt, &r.Source); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// ============ Calendar Events ============

// CreateEvent 创建日历事件
func (db *DB) CreateEvent(ctx context.Context, e *model.CalendarEvent) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO calendar_events (member_id, title, description, start_time, end_time, location, event_type, date, time, type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		e.MemberID, e.Title, e.Description, e.StartTime, e.EndTime, e.Location, e.EventType, e.Date, e.Time, e.Type)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetEvents 获取日历事件
func (db *DB) GetEvents(ctx context.Context, memberID int64) ([]model.CalendarEvent, error) {
	query := "SELECT id, member_id, title, description, start_time, end_time, location, event_type, date, time, type FROM calendar_events"
	var args []interface{}
	if memberID > 0 {
		query += " WHERE member_id = ?"
		args = append(args, memberID)
	}
	query += " ORDER BY COALESCE(start_time, date || ' ' || COALESCE(time,'00:00')) DESC"
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []model.CalendarEvent
	for rows.Next() {
		var e model.CalendarEvent
		if err := rows.Scan(&e.ID, &e.MemberID, &e.Title, &e.Description, &e.StartTime, &e.EndTime, &e.Location, &e.EventType, &e.Date, &e.Time, &e.Type); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ============ Trip Plans ============

// CreateTripPlan 创建出行计划
func (db *DB) CreateTripPlan(ctx context.Context, t *model.TripPlan) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO trip_plans (title, destination, start_date, end_date, status, plan_json, created_by, members_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		t.Title, t.Destination, t.StartDate, t.EndDate, t.Status, t.PlanJSON, t.CreatedBy, t.MembersJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetTripPlans 获取所有出行计划
func (db *DB) GetTripPlans(ctx context.Context) ([]model.TripPlan, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, title, destination, start_date, end_date, status, plan_json, created_by, members_json FROM trip_plans ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plans []model.TripPlan
	for rows.Next() {
		var t model.TripPlan
		if err := rows.Scan(&t.ID, &t.Title, &t.Destination, &t.StartDate, &t.EndDate, &t.Status, &t.PlanJSON, &t.CreatedBy, &t.MembersJSON); err != nil {
			return nil, err
		}
		plans = append(plans, t)
	}
	return plans, rows.Err()
}

// ============ Chat Messages ============

// CreateChatMessage 创建聊天消息
func (db *DB) CreateChatMessage(ctx context.Context, m *model.ChatMessage) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO chat_messages (member_id, content, role, timestamp, session_id) VALUES (?, ?, ?, ?, ?)",
		m.MemberID, m.Content, m.Role, m.Timestamp, m.SessionID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetChatHistory 获取聊天历史
func (db *DB) GetChatHistory(ctx context.Context, sessionID string, limit int) ([]model.ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, member_id, content, role, timestamp, session_id FROM chat_messages WHERE session_id = ? ORDER BY timestamp DESC LIMIT ?",
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []model.ChatMessage
	for rows.Next() {
		var m model.ChatMessage
		if err := rows.Scan(&m.ID, &m.MemberID, &m.Content, &m.Role, &m.Timestamp, &m.SessionID); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	// 反转以得到时间正序
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, rows.Err()
}

// ============ Reminders ============

// CreateReminder 创建提醒
func (db *DB) CreateReminder(ctx context.Context, r *model.Reminder) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO reminders (member_id, title, content, remind_at, status, channel) VALUES (?, ?, ?, ?, ?, ?)",
		r.MemberID, r.Title, r.Content, r.RemindAt, r.Status, r.Channel)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetReminders 获取提醒列表
func (db *DB) GetReminders(ctx context.Context, memberID int64, status string) ([]model.Reminder, error) {
	query := "SELECT id, member_id, title, content, remind_at, status, channel FROM reminders"
	var args []interface{}
	if memberID > 0 {
		query += " WHERE member_id = ?"
		args = append(args, memberID)
		if status != "" {
			query += " AND status = ?"
			args = append(args, status)
		}
	} else if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY remind_at ASC"
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reminders []model.Reminder
	for rows.Next() {
		var r model.Reminder
		if err := rows.Scan(&r.ID, &r.MemberID, &r.Title, &r.Content, &r.RemindAt, &r.Status, &r.Channel); err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}
	return reminders, rows.Err()
}

// ============ Device Data ============

// CreateDeviceData 创建设备数据
func (db *DB) CreateDeviceData(ctx context.Context, d *model.DeviceData) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO device_data (member_id, device_type, data_json, received_at) VALUES (?, ?, ?, ?)",
		d.MemberID, d.DeviceType, d.DataJSON, d.ReceivedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetDeviceData 获取设备数据
func (db *DB) GetDeviceData(ctx context.Context, memberID int64, deviceType string) ([]model.DeviceData, error) {
	query := "SELECT id, member_id, device_type, data_json, received_at FROM device_data"
	var args []interface{}
	if memberID > 0 {
		query += " WHERE member_id = ?"
		args = append(args, memberID)
		if deviceType != "" {
			query += " AND device_type = ?"
			args = append(args, deviceType)
		}
	} else if deviceType != "" {
		query += " WHERE device_type = ?"
		args = append(args, deviceType)
	}
	query += " ORDER BY received_at DESC"
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []model.DeviceData
	for rows.Next() {
		var d model.DeviceData
		if err := rows.Scan(&d.ID, &d.MemberID, &d.DeviceType, &d.DataJSON, &d.ReceivedAt); err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, rows.Err()
}

// ============ Feedback ============

// CreateFeedback 创建反馈记录
func (db *DB) CreateFeedback(ctx context.Context, f *model.FeedbackRecord) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO feedback_records (member_id, item_name, item_type, rating, comment, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		f.MemberID, f.ItemName, f.ItemType, f.Rating, f.Comment, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetFeedbackList 获取反馈列表
func (db *DB) GetFeedbackList(ctx context.Context, limit int) ([]model.FeedbackRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, member_id, item_name, item_type, rating, comment, created_at FROM feedback_records ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.FeedbackRecord
	for rows.Next() {
		var f model.FeedbackRecord
		if err := rows.Scan(&f.ID, &f.MemberID, &f.ItemName, &f.ItemType, &f.Rating, &f.Comment, &f.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

// ============ Points Records (新增) ============

// AddPointsRecord 添加积分记录
func (db *DB) AddPointsRecord(ctx context.Context, memberName, ptsType, typeLabel, title string, points int) error {
	_, err := db.conn.ExecContext(ctx,
		"INSERT INTO points_records (member_name, pts_type, type_label, title, points) VALUES (?, ?, ?, ?, ?)",
		memberName, ptsType, typeLabel, title, points)
	return err
}

// GetPointsByMember 获取成员总积分（按类型分组）
func (db *DB) GetPointsByMember(ctx context.Context, memberName string) (total, sport, health, labor int, err error) {
	err = db.conn.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(points),0), COALESCE(SUM(CASE WHEN pts_type='exercise' THEN points ELSE 0 END),0), COALESCE(SUM(CASE WHEN pts_type='health' THEN points ELSE 0 END),0), COALESCE(SUM(CASE WHEN pts_type='chorse' THEN points ELSE 0 END),0) FROM points_records WHERE member_name=?",
		memberName).Scan(&total, &sport, &health, &labor)
	return
}

// GetPointsRanking 获取积分排行榜
func (db *DB) GetPointsRanking(ctx context.Context, limit int) ([]struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
}, error) {
	if limit <= 0 {
		limit = 20
	}
	// LEFT JOIN family_members：0 积分成员也上榜
	rows, err := db.conn.QueryContext(ctx,
		`SELECT fm.name, COALESCE(SUM(pr.points), 0) AS total
		 FROM family_members fm
		 LEFT JOIN points_records pr ON pr.member_name = fm.name
		 GROUP BY fm.id, fm.name
		 ORDER BY total DESC, fm.id ASC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []struct {
		Name  string `json:"name"`
		Total int    `json:"total"`
	}
	for rows.Next() {
		var r struct {
			Name  string `json:"name"`
			Total int    `json:"total"`
		}
		if err := rows.Scan(&r.Name, &r.Total); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetRecentPointsRecords 获取最近积分记录
func (db *DB) GetRecentPointsRecords(ctx context.Context, limit int) ([]struct {
	ID     int64  `json:"id"`
	Time   string `json:"time"`
	Member string `json:"member"`
	Type   string `json:"type"`
	Title  string `json:"title"`
	Points int    `json:"points"`
}, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, strftime('%H:%M', created_at), member_name, type_label, title, points FROM points_records ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []struct {
		ID     int64  `json:"id"`
		Time   string `json:"time"`
		Member string `json:"member"`
		Type   string `json:"type"`
		Title  string `json:"title"`
		Points int    `json:"points"`
	}
	for rows.Next() {
		var r struct {
			ID     int64  `json:"id"`
			Time   string `json:"time"`
			Member string `json:"member"`
			Type   string `json:"type"`
			Title  string `json:"title"`
			Points int    `json:"points"`
		}
		if err := rows.Scan(&r.ID, &r.Time, &r.Member, &r.Type, &r.Title, &r.Points); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// DeletePointsRecord 删除积分记录
func (db *DB) DeletePointsRecord(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM points_records WHERE id=?", id)
	return err
}

// GetSetting 读取家庭设置（key-value），不存在返回空字符串
func (db *DB) GetSetting(ctx context.Context, key string) string {
	var val string
	err := db.conn.QueryRowContext(ctx, "SELECT value FROM family_settings WHERE key=?", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

// SetSetting 写入家庭设置（upsert）
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO family_settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value)
	return err
}

// GetWeeklyPointsSum 获取本周（周一至今）积分总和
func (db *DB) GetWeeklyPointsSum(ctx context.Context) (int, error) {
	// date('now','weekday 0','-6 days') 返回本周一（SQLite weekday 0=Sunday，-6 days 回退到周一）
	var sum int
	err := db.conn.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(points),0) FROM points_records WHERE created_at >= date('now','weekday 0','-6 days')").Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// ============ Chorse (新增) ============

// CreateChorseTask 创建家务任务
func (db *DB) CreateChorseTask(ctx context.Context, task *model.ChorseTaskDB) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO chorse_tasks (name, icon, category, difficulty, points, duration, description, enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		task.Name, task.Icon, task.Category, task.Difficulty, task.Points, task.Duration, task.Description, task.Enabled)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListChorseTasks 列出可用家务任务（仅启用的）
func (db *DB) ListChorseTasks(ctx context.Context) ([]model.ChorseTaskDB, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, name, icon, category, difficulty, points, duration, description, enabled FROM chorse_tasks WHERE enabled=1 AND is_active=1 ORDER BY category, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []model.ChorseTaskDB
	for rows.Next() {
		var t model.ChorseTaskDB
		if err := rows.Scan(&t.ID, &t.Name, &t.Icon, &t.Category, &t.Difficulty, &t.Points, &t.Duration, &t.Description, &t.Enabled); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ListAllChorseTasks 查询所有任务（管理用，含禁用的）
func (db *DB) ListAllChorseTasks(ctx context.Context) ([]model.ChorseTaskDB, error) {
	rows, err := db.conn.QueryContext(ctx, "SELECT id, name, icon, category, difficulty, points, duration, description, enabled FROM chorse_tasks ORDER BY category, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []model.ChorseTaskDB
	for rows.Next() {
		var t model.ChorseTaskDB
		if err := rows.Scan(&t.ID, &t.Name, &t.Icon, &t.Category, &t.Difficulty, &t.Points, &t.Duration, &t.Description, &t.Enabled); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ToggleChorseTask 启用/禁用任务
func (db *DB) ToggleChorseTask(ctx context.Context, id int64, enabled bool) error {
	_, err := db.conn.ExecContext(ctx, "UPDATE chorse_tasks SET enabled=? WHERE id=?", enabled, id)
	return err
}

// CreateChorseClaim 创建认领记录
func (db *DB) CreateChorseClaim(ctx context.Context, claim *model.ChorseClaimDB) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO chorse_claims (task_id, task_name, task_icon, member_id, member_name, deadline, status, points, verifier_id, verifier_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		claim.TaskID, claim.TaskName, claim.TaskIcon, claim.MemberID, claim.MemberName, claim.Deadline, claim.Status, claim.Points, claim.VerifierID, claim.VerifierName)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CompleteChorseClaim 标记认领完成
func (db *DB) CompleteChorseClaim(ctx context.Context, claimID int64) error {
	_, err := db.conn.ExecContext(ctx, "UPDATE chorse_claims SET status='completed' WHERE id=? AND status='pending'", claimID)
	return err
}

// ConfirmChorseClaim 确认认领完成
func (db *DB) ConfirmChorseClaim(ctx context.Context, claimID, confirmerID int64, confirmerName string) (*model.ChorseClaimDB, error) {
	var claim model.ChorseClaimDB
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx,
		"SELECT id, task_id, task_name, task_icon, member_id, member_name, claimed_at, deadline, status, points, verifier_id, verifier_name, COALESCE(confirmed_by, '') AS confirmed_by, confirmed_at FROM chorse_claims WHERE id=? AND status='completed'",
		claimID).Scan(&claim.ID, &claim.TaskID, &claim.TaskName, &claim.TaskIcon, &claim.MemberID, &claim.MemberName, &claim.ClaimedAt, &claim.Deadline, &claim.Status, &claim.Points, &claim.VerifierID, &claim.VerifierName, &claim.ConfirmedBy, &claim.ConfirmedAt)
	if err != nil {
		return nil, fmt.Errorf("认领记录不存在或状态不允许确认: %w", err)
	}

	_, err = tx.ExecContext(ctx, "UPDATE chorse_claims SET status='confirmed', confirmed_by=?, confirmed_at=datetime('now') WHERE id=?",
		confirmerName, claimID)
	if err != nil {
		return nil, err
	}

	// 联动积分
	_, err = tx.ExecContext(ctx,
		"INSERT INTO points_records (member_name, pts_type, type_label, title, points) VALUES (?, 'chorse', '劳动', ?, ?)",
		claim.MemberName, claim.TaskName, claim.Points)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	claim.Status = "confirmed"
	return &claim, nil
}

// GetPendingChorseClaims 获取待处理认领（pending + completed）
func (db *DB) GetPendingChorseClaims(ctx context.Context) ([]model.ChorseClaimDB, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, task_id, task_name, task_icon, member_id, member_name, claimed_at, deadline, status, points, verifier_id, verifier_name, COALESCE(confirmed_by, '') AS confirmed_by, confirmed_at
		 FROM chorse_claims WHERE status IN ('pending','completed') ORDER BY
		 CASE WHEN status='pending' THEN 0 ELSE 1 END,
		 claimed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var claims []model.ChorseClaimDB
	for rows.Next() {
		var c model.ChorseClaimDB
		if err := rows.Scan(&c.ID, &c.TaskID, &c.TaskName, &c.TaskIcon, &c.MemberID, &c.MemberName, &c.ClaimedAt, &c.Deadline, &c.Status, &c.Points, &c.VerifierID, &c.VerifierName, &c.ConfirmedBy, &c.ConfirmedAt); err != nil {
			return nil, err
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}

// GetTodayCompletedClaims 获取今日已完成的认领（大屏用）
func (db *DB) GetTodayCompletedClaims(ctx context.Context) ([]model.ChorseClaimDB, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, task_id, task_name, task_icon, member_id, member_name, claimed_at, deadline, status, points, verifier_id, verifier_name, COALESCE(confirmed_by, '') AS confirmed_by, confirmed_at FROM chorse_claims WHERE status='confirmed' AND date(confirmed_at)=date('now') ORDER BY confirmed_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var claims []model.ChorseClaimDB
	for rows.Next() {
		var c model.ChorseClaimDB
		if err := rows.Scan(&c.ID, &c.TaskID, &c.TaskName, &c.TaskIcon, &c.MemberID, &c.MemberName, &c.ClaimedAt, &c.Deadline, &c.Status, &c.Points, &c.VerifierID, &c.VerifierName, &c.ConfirmedBy, &c.ConfirmedAt); err != nil {
			return nil, err
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}

// ============ Weekend Proposals (新增) ============

// CreateWeekendProposal 创建周末出行方案
func (db *DB) CreateWeekendProposal(ctx context.Context, p *model.WeekendProposalDB) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO weekend_proposals (title, description, icon, category, tags_json, duration, cost, difficulty, suitable_for, weather_req, tips, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		p.Title, p.Description, p.Icon, p.Category, p.TagsJSON, p.Duration, p.Cost, p.Difficulty, p.SuitableFor, p.WeatherReq, p.Tips, p.CreatedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListWeekendProposals 列出周末方案
func (db *DB) ListWeekendProposals(ctx context.Context) ([]model.WeekendProposalDB, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, title, description, icon, category, tags_json, duration, cost, difficulty, suitable_for, weather_req, tips, created_by FROM weekend_proposals ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proposals []model.WeekendProposalDB
	for rows.Next() {
		var p model.WeekendProposalDB
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Icon, &p.Category, &p.TagsJSON, &p.Duration, &p.Cost, &p.Difficulty, &p.SuitableFor, &p.WeatherReq, &p.Tips, &p.CreatedBy); err != nil {
			return nil, err
		}
		proposals = append(proposals, p)
	}
	return proposals, rows.Err()
}

// GetWeekendProposalByID 单个方案详情
func (db *DB) GetWeekendProposalByID(ctx context.Context, id int64) (*model.WeekendProposalDB, error) {
	var p model.WeekendProposalDB
	err := db.conn.QueryRowContext(ctx,
		"SELECT id, title, description, icon, category, tags_json, duration, cost, difficulty, suitable_for, weather_req, tips, created_by FROM weekend_proposals WHERE id=?", id).
		Scan(&p.ID, &p.Title, &p.Description, &p.Icon, &p.Category, &p.TagsJSON, &p.Duration, &p.Cost, &p.Difficulty, &p.SuitableFor, &p.WeatherReq, &p.Tips, &p.CreatedBy)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateWeekendProposal 更新方案
func (db *DB) UpdateWeekendProposal(ctx context.Context, p *model.WeekendProposalDB) error {
	res, err := db.conn.ExecContext(ctx,
		`UPDATE weekend_proposals SET title=?, description=?, icon=?, category=?, tags_json=?, duration=?, cost=?, difficulty=?, suitable_for=?, weather_req=?, tips=? WHERE id=?`,
		p.Title, p.Description, p.Icon, p.Category, p.TagsJSON, p.Duration, p.Cost,
		p.Difficulty, p.SuitableFor, p.WeatherReq, p.Tips, p.ID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("方案不存在")
	}
	return nil
}

// DeleteWeekendProposal 删除方案（级联删除投票记录）
func (db *DB) DeleteWeekendProposal(ctx context.Context, id int64) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM weekend_votes WHERE proposal_id=?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM weekend_proposals WHERE id=?`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("方案不存在")
	}
	return tx.Commit()
}

// AddWeekendVote 添加投票
func (db *DB) AddWeekendVote(ctx context.Context, proposalID, memberID int64, memberName string) error {
	_, err := db.conn.ExecContext(ctx,
		"INSERT INTO weekend_votes (proposal_id, member_id, member_name) VALUES (?, ?, ?)",
		proposalID, memberID, memberName)
	return err
}

// RemoveWeekendVote 取消投票
func (db *DB) RemoveWeekendVote(ctx context.Context, memberID int64) (proposalID int64, err error) {
	err = db.conn.QueryRowContext(ctx,
		"SELECT proposal_id FROM weekend_votes WHERE member_id=? LIMIT 1", memberID).Scan(&proposalID)
	if err != nil {
		return 0, err
	}
	_, err = db.conn.ExecContext(ctx, "DELETE FROM weekend_votes WHERE member_id=?", memberID)
	return
}

// GetWeekendVoteResults 获取投票结果统计
func (db *DB) GetWeekendVoteResults(ctx context.Context) ([]struct {
	ProposalID   int64    `json:"proposal_id"`
	ProposalName string   `json:"proposal_name"`
	VoteCount    int      `json:"vote_count"`
	Voters       []string `json:"voters"`
}, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT wp.id, wp.title, COUNT(wv.id) as vote_count, GROUP_CONCAT(wv.member_name) as voters FROM weekend_proposals wp LEFT JOIN weekend_votes wv ON wp.id=wv.proposal_id GROUP BY wp.id ORDER BY vote_count DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []struct {
		ProposalID   int64    `json:"proposal_id"`
		ProposalName string   `json:"proposal_name"`
		VoteCount    int      `json:"vote_count"`
		Voters       []string `json:"voters"`
	}
	for rows.Next() {
		var r struct {
			ProposalID   int64    `json:"proposal_id"`
			ProposalName string   `json:"proposal_name"`
			VoteCount    int      `json:"vote_count"`
			Voters       []string `json:"voters"`
		}
		var votersStr *string
		if err := rows.Scan(&r.ProposalID, &r.ProposalName, &r.VoteCount, &votersStr); err != nil {
			return nil, err
		}
		if votersStr != nil && *votersStr != "" {
			r.Voters = splitComma(*votersStr)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ============ Health Data Cache ============

// UpsertHealthDataCache 写入或更新健康数据缓存
func (db *DB) UpsertHealthDataCache(ctx context.Context, cache *model.HealthDataCache) error {
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO health_data_cache (member_id, date, steps, heart_rate, sleep_hours, sleep_score, blood_pressure_sys, blood_pressure_dia, weight, height, stress, spo2, body_battery, calories, source, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(member_id, date) DO UPDATE SET
		 steps=excluded.steps, heart_rate=excluded.heart_rate, sleep_hours=excluded.sleep_hours, sleep_score=excluded.sleep_score,
		 blood_pressure_sys=excluded.blood_pressure_sys, blood_pressure_dia=excluded.blood_pressure_dia, weight=excluded.weight, height=excluded.height,
		 stress=excluded.stress, spo2=excluded.spo2, body_battery=excluded.body_battery, calories=excluded.calories, source=excluded.source, synced_at=excluded.synced_at`,
		cache.MemberID, cache.Date, cache.Steps, cache.HeartRate, cache.SleepHours, cache.SleepScore,
		cache.BloodPressureSys, cache.BloodPressureDia, cache.Weight, cache.Height, cache.Stress, cache.SpO2,
		cache.BodyBattery, cache.Calories, cache.Source, cache.SyncedAt)
	return err
}

// GetHealthDataCache 获取健康数据缓存
func (db *DB) GetHealthDataCache(ctx context.Context, memberID int64, date string) (*model.HealthDataCache, error) {
	row := db.conn.QueryRowContext(ctx,
		"SELECT id, member_id, date, steps, heart_rate, sleep_hours, sleep_score, blood_pressure_sys, blood_pressure_dia, weight, height, stress, spo2, body_battery, calories, source, synced_at FROM health_data_cache WHERE member_id=? AND date=?",
		memberID, date)
	var c model.HealthDataCache
	err := row.Scan(&c.ID, &c.MemberID, &c.Date, &c.Steps, &c.HeartRate, &c.SleepHours, &c.SleepScore,
		&c.BloodPressureSys, &c.BloodPressureDia, &c.Weight, &c.Height, &c.Stress, &c.SpO2, &c.BodyBattery, &c.Calories, &c.Source, &c.SyncedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ============ Data Source Config ============

// SaveDataSourceConfig 保存数据源配置
func (db *DB) SaveDataSourceConfig(ctx context.Context, cfg *model.DataSourceConfig) (int64, error) {
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO data_source_config (member_id, member_name, source_type, api_key, api_secret, user_id, is_active, last_sync_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		cfg.MemberID, cfg.MemberName, cfg.SourceType, cfg.APIKey, cfg.APISecret, cfg.UserID, cfg.IsActive, cfg.LastSyncAt, cfg.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetDataSourceConfigs 获取数据源配置列表
func (db *DB) GetDataSourceConfigs(ctx context.Context) ([]model.DataSourceConfig, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, member_id, member_name, source_type, api_key, api_secret, user_id, is_active, last_sync_at, created_at FROM data_source_config ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []model.DataSourceConfig
	for rows.Next() {
		var c model.DataSourceConfig
		if err := rows.Scan(&c.ID, &c.MemberID, &c.MemberName, &c.SourceType, &c.APIKey, &c.APISecret, &c.UserID, &c.IsActive, &c.LastSyncAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// splitComma 辅助：逗号分割字符串
func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range []byte(s) {
		result = append(result, string(part))
	}
	// SQLite GROUP_CONCAT 用逗号分隔
	parts := make([]string, 0)
	current := ""
	for _, ch := range s {
		if ch == ',' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// createHealthMetricTables 创建健康指标索引
func (db *DB) createHealthMetricTables() error {
	_, err := db.conn.Exec(`
	CREATE INDEX IF NOT EXISTS idx_health_metrics_member ON health_metrics(member_name, family_id);
	`)
	return err
}

// seedChorseTasks 预置 10 种常见家务任务（PRD M-17 修复）
func (db *DB) seedChorseTasks(ctx context.Context) error {
	// 检查是否已有任务
	var count int
	err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM chorse_tasks").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // 已有数据，跳过
	}

	tasks := []model.ChorseTaskDB{
		{Name: "洗碗", Icon: "🍽️", Category: "厨房", Difficulty: "简单", Points: 10, Duration: "30分钟", Description: "饭后清洗餐具、擦拭灶台", Enabled: true},
		{Name: "扫地拖地", Icon: "🧹", Category: "清洁", Difficulty: "简单", Points: 15, Duration: "30分钟", Description: "全屋扫地和拖地", Enabled: true},
		{Name: "倒垃圾", Icon: "🗑️", Category: "清洁", Difficulty: "简单", Points: 5, Duration: "10分钟", Description: "收集并丢弃生活垃圾", Enabled: true},
		{Name: "洗衣服", Icon: "👕", Category: "洗衣", Difficulty: "简单", Points: 15, Duration: "20分钟", Description: "分类投放、启动洗衣机、晾晒", Enabled: true},
		{Name: "整理房间", Icon: "🧹", Category: "整理", Difficulty: "中等", Points: 20, Duration: "45分钟", Description: "收拾个人物品、整理桌面和床铺", Enabled: true},
		{Name: "做饭", Icon: "🍳", Category: "厨房", Difficulty: "中等", Points: 25, Duration: "60分钟", Description: "准备食材、烹饪、收拾厨房", Enabled: true},
		{Name: "擦窗户", Icon: "🪟", Category: "清洁", Difficulty: "中等", Points: 20, Duration: "40分钟", Description: "擦拭玻璃窗、窗框和窗台", Enabled: true},
		{Name: "浇花", Icon: "🌸", Category: "园艺", Difficulty: "简单", Points: 5, Duration: "10分钟", Description: "给室内外植物浇水", Enabled: true},
		{Name: "遛狗", Icon: "🐕", Category: "宠物", Difficulty: "简单", Points: 15, Duration: "30分钟", Description: "带宠物出门散步", Enabled: true},
		{Name: "整理书桌", Icon: "📚", Category: "整理", Difficulty: "简单", Points: 10, Duration: "20分钟", Description: "整理书桌上的书本和文具", Enabled: true},
	}

	for _, t := range tasks {
		if _, err := db.CreateChorseTask(ctx, &t); err != nil {
			log.Printf("[WARN] 预置任务失败 %s: %v", t.Name, err)
		}
	}
	log.Printf("[INFO] 已预置 %d 种家务任务", len(tasks))
	return nil
}

// seedAdminUser 创建默认管理员账户
func (db *DB) seedAdminUser(ctx context.Context) error {
	var count int
	err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	// 默认管理员: admin / admin123
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	res, err := db.conn.ExecContext(ctx,
		"INSERT INTO users (username, password_hash, role, name, family_id) VALUES (?, ?, ?, ?, ?)",
		"admin", string(hash), "admin", "管理员", 1)
	if err != nil {
		return err
	}
	userID, _ := res.LastInsertId()
	// 同时创建对应 family_member，user_id 关联
	_, err = db.conn.ExecContext(ctx,
		"INSERT INTO family_members (user_id, name, role, age, preferences_json) VALUES (?, ?, ?, ?, ?)",
		userID, "管理员", "admin", 0, "")
	if err != nil {
		log.Printf("[WARN] 创建默认管理员成员失败: %v", err)
	}
	log.Println("[INFO] 已创建默认管理员账户: admin / admin123（请及时修改密码）")
	return nil
}

// ResetPassword 重置用户密码
func (db *DB) ResetPassword(ctx context.Context, username, newHash string) error {
	res, err := db.conn.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE username = ?", newHash, username)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

// ============ Health Metrics (自定义指标) ============

// AddHealthMetricOld 旧版添加健康指标（保留向后兼容）
func (db *DB) AddHealthMetricOld(ctx context.Context, familyID int, memberName, label, unit, icon string, value float64) error {
	_, err := db.conn.ExecContext(ctx,
		"INSERT INTO health_metrics (family_id, member_name, label, value, unit, icon) VALUES (?, ?, ?, ?, ?, ?)",
		familyID, memberName, label, value, unit, icon)
	return err
}

// GetHealthMetrics 获取成员的健康指标（取每个 label 最新一条）
func (db *DB) GetHealthMetrics(ctx context.Context, familyID int) ([]map[string]interface{}, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT hm.member_name, hm.label, hm.value, hm.unit, hm.icon, hm.status, hm.trend, hm.recorded_at
		 FROM health_metrics hm
		 INNER JOIN (
		     SELECT member_name, label, MAX(id) AS max_id
		     FROM health_metrics
		     WHERE family_id = ?
		     GROUP BY member_name, label
		 ) latest ON hm.id = latest.max_id
		 ORDER BY hm.member_name, hm.recorded_at DESC`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name, label, unit, icon, status, trend string
		var value float64
		var recorded time.Time
		if err := rows.Scan(&name, &label, &value, &unit, &icon, &status, &trend, &recorded); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"member_name": name,
			"label":       label,
			"value":       value,
			"unit":        unit,
			"icon":        icon,
			"status":      status,
			"trend":       trend,
			"recorded_at": recorded.Format("2006-01-02 15:04"),
		})
	}
	return results, nil
}

// AddHealthMetric 添加自定义健康指标
func (db *DB) AddHealthMetric(ctx context.Context, m *model.HealthMetric) (int64, error) {
	result, err := db.conn.ExecContext(ctx,
		`INSERT INTO health_metrics (member_name, label, value, unit, icon, status, trend, family_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		m.MemberName, m.Label, m.Value, m.Unit, m.Icon, m.Status, m.Trend)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetHealthMetricsByMember 获取成员的自定义指标
func (db *DB) GetHealthMetricsByMember(ctx context.Context, memberName string) ([]model.HealthMetric, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_name, label, value, unit, icon, status, trend, created_at
		 FROM health_metrics WHERE member_name = ? AND family_id = 1 ORDER BY created_at DESC`,
		memberName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metrics []model.HealthMetric
	for rows.Next() {
		var m model.HealthMetric
		if err := rows.Scan(&m.ID, &m.MemberName, &m.Label, &m.Value, &m.Unit, &m.Icon, &m.Status, &m.Trend, &m.CreatedAt); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// GetAllHealthMetrics 获取所有家庭的健康指标
func (db *DB) GetAllHealthMetrics(ctx context.Context) ([]model.HealthMetric, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, member_name, label, value, unit, icon, status, trend, created_at
		 FROM health_metrics WHERE family_id = 1 ORDER BY member_name, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var metrics []model.HealthMetric
	for rows.Next() {
		var m model.HealthMetric
		if err := rows.Scan(&m.ID, &m.MemberName, &m.Label, &m.Value, &m.Unit, &m.Icon, &m.Status, &m.Trend, &m.CreatedAt); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// DeleteHealthMetric 删除自定义指标
func (db *DB) DeleteHealthMetric(ctx context.Context, id int64) error {
	_, err := db.conn.ExecContext(ctx, `DELETE FROM health_metrics WHERE id = ?`, id)
	return err
}