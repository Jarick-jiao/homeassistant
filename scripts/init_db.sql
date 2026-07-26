-- HomeMate v3.0 数据库初始化脚本
-- SQLite

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'guest',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS family_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'guest',
    age INTEGER,
    preferences_json TEXT,
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
    location TEXT,
    event_type TEXT,
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
    FOREIGN KEY (created_by) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    member_id INTEGER,
    content TEXT NOT NULL,
    role TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
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
    rating INTEGER,
    comment TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES family_members(id)
);

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

-- v3.9.13: 每个成员同一数据源类型仅允许一条配置，防止编辑时新增重复行
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_source_config_member_type
ON data_source_config(member_id, source_type);
