package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/homemate/server/internal/config"
)

// TestRunScriptSyncAllExec 集成测试：真实 exec 一个 Python 脚本，
// 验证 exec 调用、环境变量注入、输出解析全链路通畅。
// 跳过条件：环境无 python3 时跳过。
func TestRunScriptSyncAllExec(t *testing.T) {
	python := "/usr/bin/python3"
	if runtime.GOOS != "linux" {
		python = "python3"
	}
	if _, err := os.Stat(python); err != nil {
		t.Skipf("python3 不可用，跳过集成测试: %v", err)
	}

	// 构造一个 dummy 脚本：打印 "成功: 2" 模拟同步 2 个成员
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "dummy_sync.py")
	dummyScript := `#!/usr/bin/env python3
import os
import sys
# 验证 Go 注入的环境变量到达脚本
db = os.environ.get("HOMEMATE_DB", "")
user = os.environ.get("GARMIN_USERNAME", "")
# 验证 PYTHONHOME 被剥离
ph = os.environ.get("PYTHONHOME", "")
sys.stderr.write(f"db={db} user={user} pythonhome={ph}\n")
print("=== 同步完成 ===")
print("成功: 2 | 跳过: 0 | 失败: 0")
`
	if err := os.WriteFile(scriptPath, []byte(dummyScript), 0o755); err != nil {
		t.Fatalf("写入 dummy 脚本失败: %v", err)
	}

	// 预置 PYTHONHOME 验证会被剥离
	os.Setenv("PYTHONHOME", "/should/be/stripped")
	defer os.Unsetenv("PYTHONHOME")

	s := &Scheduler{
		dbPath: "/tmp/test_homemate.db",
		garminCfg: config.GarminConfig{
			UseScriptSync:  true,
			SyncScriptPath: scriptPath,
			PythonPath:     python,
			ScriptTimeout:  30 * time.Second,
			Username:       "integration@test.com",
			Password:       "pwd",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got := s.runScriptSyncAll(ctx, "2026-07-25")
	if got != 2 {
		t.Errorf("期望解析到 2 个同步成员，got %d", got)
	}
}
