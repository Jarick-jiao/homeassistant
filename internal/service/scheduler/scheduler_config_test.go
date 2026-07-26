package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/homemate/server/internal/config"
	"github.com/homemate/server/internal/store"
)

// newTestScheduler 构造带临时 SQLite 的调度器，预置 3 个任务用于配置测试
func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	tmpFile := t.TempDir() + "/test_cfg.db"
	db, err := store.InitDB(config.DatabaseConfig{Path: tmpFile, WALMode: false, MaxOpen: 1, MaxIdle: 1})
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Scheduler{
		db:         db,
		stopCh:     make(chan struct{}),
		lastRuns:   make(map[string]time.Time),
		lastStatus: make(map[string]string),
		taskRegistry: []taskMeta{
			{Task: "health_sync", Name: "健康数据同步", Interval: 6 * time.Hour, Enabled: true},
			{Task: "ai_analysis", Name: "AI 健康分析", Interval: 24 * time.Hour, Enabled: true},
			{Task: "cleanup", Name: "历史数据清理", Interval: 24 * time.Hour, Enabled: true},
		},
	}
}

// TestStatusReturnsInterval 验证 Status() 返回 interval 字段
func TestStatusReturnsInterval(t *testing.T) {
	s := newTestScheduler(t)
	res := s.Status()
	tasks, ok := res["tasks"].([]map[string]interface{})
	if !ok {
		t.Fatalf("tasks 类型错误: %T", res["tasks"])
	}
	if len(tasks) == 0 {
		t.Fatal("tasks 为空")
	}
	interval, ok := tasks[0]["interval"]
	if !ok {
		t.Fatal("Status 缺少 interval 字段")
	}
	if interval != "6h0m0s" {
		t.Errorf("interval 期望 6h0m0s, 实际 %v", interval)
	}
}

// TestUpdateTaskConfigDisable 验证禁用任务立即生效并持久化
func TestUpdateTaskConfigDisable(t *testing.T) {
	s := newTestScheduler(t)
	if !s.isTaskEnabled("health_sync") {
		t.Fatal("初始应为启用")
	}
	off := false
	if err := s.UpdateTaskConfig("health_sync", &off, nil); err != nil {
		t.Fatalf("UpdateTaskConfig 失败: %v", err)
	}
	if s.isTaskEnabled("health_sync") {
		t.Error("禁用后 isTaskEnabled 应返回 false")
	}
	if v := s.db.GetSetting(context.Background(), "scheduler.health_sync.enabled"); v != "false" {
		t.Errorf("持久化 enabled 期望 false, 实际 %s", v)
	}
}

// TestUpdateTaskConfigInterval 验证间隔持久化
func TestUpdateTaskConfigInterval(t *testing.T) {
	s := newTestScheduler(t)
	d := 30 * time.Minute
	if err := s.UpdateTaskConfig("cleanup", nil, &d); err != nil {
		t.Fatalf("UpdateTaskConfig 失败: %v", err)
	}
	if v := s.db.GetSetting(context.Background(), "scheduler.cleanup.interval"); v != "30m0s" {
		t.Errorf("持久化 interval 期望 30m0s, 实际 %s", v)
	}
}

// TestUpdateTaskConfigUnknownTask 验证未知任务返回错误
func TestUpdateTaskConfigUnknownTask(t *testing.T) {
	s := newTestScheduler(t)
	on := true
	if err := s.UpdateTaskConfig("no_such_task", &on, nil); err == nil {
		t.Error("未知任务应返回错误")
	}
}

// TestLoadTaskConfigOverrides 验证启动时从 settings 读取覆盖
func TestLoadTaskConfigOverrides(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()
	s.db.SetSetting(ctx, "scheduler.cleanup.enabled", "false")
	s.db.SetSetting(ctx, "scheduler.health_sync.interval", "30m")
	cfg := DefaultTaskConfig()
	s.loadTaskConfigOverrides(&cfg)
	if s.isTaskEnabled("cleanup") {
		t.Error("loadTaskConfigOverrides 应将 cleanup 置为禁用")
	}
	if cfg.HealthSyncInterval != 30*time.Minute {
		t.Errorf("HealthSyncInterval 期望 30m, 实际 %v", cfg.HealthSyncInterval)
	}
}
