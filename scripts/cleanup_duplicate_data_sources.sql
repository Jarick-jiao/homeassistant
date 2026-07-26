-- HomeMate v3.9.13: data_source_config 重复行一次性清理脚本
--
-- 背景：旧版 SaveDataSourceConfig 为纯 INSERT 逻辑，每次编辑成员数据源都会新增一行，
-- 导致同一 (member_id, source_type) 存在多条配置。新版已改为 UPSERT，本脚本用于
-- 清理历史已产生的重复数据，并建立唯一索引防止再次复发。
--
-- 用法（生产库手动执行）:
--   sqlite3 /path/to/homemate.db < scripts/cleanup_duplicate_data_sources.sql
--
-- 安全性：保留每组 (member_id, source_type) 的 MAX(id)（最新写入）；member_id 或
-- source_type 为 NULL 的脏行不在约束范围内，不会被删除。
-- 幂等：无重复行时 DELETE 为 no-op；CREATE UNIQUE INDEX IF NOT EXISTS 幂等。

BEGIN TRANSACTION;

-- 1. 清理重复行：每组 (member_id, source_type) 仅保留 MAX(id)
DELETE FROM data_source_config
WHERE id NOT IN (
    SELECT MAX(id)
    FROM data_source_config
    WHERE member_id IS NOT NULL AND source_type IS NOT NULL
    GROUP BY member_id, source_type
);

-- 2. 建立唯一索引，确保同一成员同一数据源类型只能有一条配置
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_source_config_member_type
ON data_source_config(member_id, source_type);

COMMIT;

-- 3. 验证（可选，手动执行查看结果）
-- SELECT member_id, source_type, COUNT(*) AS cnt
-- FROM data_source_config
-- WHERE member_id IS NOT NULL AND source_type IS NOT NULL
-- GROUP BY member_id, source_type
-- HAVING COUNT(*) > 1;
-- 预期：返回空集（无重复）
