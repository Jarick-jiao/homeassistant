package scheduler

import (
	"os"
	"strings"
	"testing"

	"github.com/homemate/server/internal/config"
)

// TestBuildScriptEnv 验证环境变量构造：剥离 PYTHONHOME/PYTHONPATH，注入 HOMEMATE_DB 和凭证
func TestBuildScriptEnv(t *testing.T) {
	// 预置可能污染的环境变量
	os.Setenv("PYTHONHOME", "/opt/conda")
	os.Setenv("PYTHONPATH", "/some/path")
	defer os.Unsetenv("PYTHONHOME")
	defer os.Unsetenv("PYTHONPATH")

	s := &Scheduler{
		dbPath: "./homemate.db",
		garminCfg: config.GarminConfig{
			Username: "tester@example.com",
			Password: "secret",
		},
	}

	env := s.buildScriptEnv("/abs/homemate.db")

	// PYTHONHOME / PYTHONPATH 必须被剥离
	for _, e := range env {
		if strings.HasPrefix(e, "PYTHONHOME=") {
			t.Fatalf("PYTHONHOME 未被剥离: %s", e)
		}
		if strings.HasPrefix(e, "PYTHONPATH=") {
			t.Fatalf("PYTHONPATH 未被剥离: %s", e)
		}
	}

	// HOMEMATE_DB 必须被注入为绝对路径
	foundDB := false
	foundUser := false
	foundPass := false
	for _, e := range env {
		if e == "HOMEMATE_DB=/abs/homemate.db" {
			foundDB = true
		}
		if e == "GARMIN_USERNAME=tester@example.com" {
			foundUser = true
		}
		if e == "GARMIN_PASSWORD=secret" {
			foundPass = true
		}
	}
	if !foundDB {
		t.Error("HOMEMATE_DB 未注入或值不正确")
	}
	if !foundUser {
		t.Error("GARMIN_USERNAME 未注入")
	}
	if !foundPass {
		t.Error("GARMIN_PASSWORD 未注入")
	}
}

// TestParseSyncedCount 验证从脚本输出解析同步条数
func TestParseSyncedCount(t *testing.T) {
	cases := []struct {
		out    string
		expect int
	}{
		{"=== 同步完成 ===\n成功: 3 | 跳过: 1 | 失败: 0", 3},
		{"成功: 1 | 跳过: 0 | 失败: 0", 1},
		{"成功:0", 0}, // 0 成员成功 → 兜底返回 1
		{"无输出", 1}, // 无法解析但视为成功
		{"成功: 12 | 跳过: 2", 12},
	}
	for i, c := range cases {
		got := parseSyncedCount(c.out)
		// "成功:0" 时兜底返回 1
		want := c.expect
		if c.expect == 0 {
			want = 1
		}
		if got != want {
			t.Errorf("case %d: parseSyncedCount(%q) = %d, want %d", i, c.out, got, want)
		}
	}
}

// TestRunScriptSyncAllMissingScript 验证脚本不存在时优雅返回 0
func TestRunScriptSyncAllMissingScript(t *testing.T) {
	s := &Scheduler{
		dbPath: "./homemate.db",
		garminCfg: config.GarminConfig{
			UseScriptSync:  true,
			SyncScriptPath: "/nonexistent/path/script.py",
			PythonPath:     "/usr/bin/python3",
		},
	}
	got := s.runScriptSyncAll(nil, "2026-07-25")
	if got != 0 {
		t.Errorf("脚本不存在时应返回 0, got %d", got)
	}
}
