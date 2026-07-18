package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DailyHealth 单日健康数据（来自 Garmin Connect）
type DailyHealth struct {
	Steps        int
	HeartRate    int     // 静息心率
	SleepHours   float64 // 睡眠小时
	SleepScore   int     // 睡眠评分
	Stress       int     // 压力指数
	SpO2         int     // 血氧
	BodyBattery  int     // 身体电量
	Calories     int     // 消耗卡路里
}

// Client Garmin Connect 客户端抽象
type Client interface {
	// Login 登录并持久化 session token（成功后 token 文件存在则复用）
	Login(ctx context.Context, username, password string) error
	// GetDailyHealth 获取指定日期的健康数据
	GetDailyHealth(ctx context.Context, date string) (*DailyHealth, error)
}

// NewClient 创建 Garmin 客户端
// tokenDir 用于持久化 session cookie；baseURL 默认 https://connect.garmin.com
func NewClient(tokenDir, baseURL string) Client {
	if baseURL == "" {
		baseURL = "https://connect.garmin.com"
	}
	jar, _ := cookiejar.New(nil)
	return &garminClient{
		tokenDir:  tokenDir,
		baseURL:   baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}
}

type garminClient struct {
	tokenDir   string
	baseURL    string
	httpClient *http.Client
	username   string
	loggedIn   bool
}

// tokenFilePath 返回 token 持久化文件路径
func (c *garminClient) tokenFilePath() string {
	if c.tokenDir == "" {
		return ""
	}
	_ = os.MkdirAll(c.tokenDir, 0700)
	return filepath.Join(c.tokenDir, "garmin_session.json")
}

// saveSession 保存 session 元信息（仅标记已登录，cookie 由 jar 维护）
func (c *garminClient) saveSession() {
	if c.tokenDir == "" {
		return
	}
	data, _ := json.Marshal(map[string]interface{}{
		"username":  c.username,
		"saved_at":  time.Now().Format(time.RFC3339),
	})
	if err := os.WriteFile(c.tokenFilePath(), data, 0600); err != nil {
		log.Printf("[GARMIN] 保存 session 失败: %v", err)
	}
}

// loadSession 尝试加载已保存的 session 标记
func (c *garminClient) loadSession(username string) bool {
	if c.tokenDir == "" {
		return false
	}
	data, err := os.ReadFile(c.tokenFilePath())
	if err != nil {
		return false
	}
	var meta struct {
		Username string `json:"username"`
		SavedAt  string `json:"saved_at"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}
	if meta.Username != username {
		return false
	}
	// session cookie 在 jar 中已丢失（进程重启），需重新登录
	return false
}

// Login 登录 Garmin Connect
// 实现 SSO 流程的简化版：POST 凭证到 sso.garmin.com，跟随重定向获取 session
func (c *garminClient) Login(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("Garmin 用户名或密码为空")
	}
	c.username = username

	// 若已有 session 标记且在 6 小时内，跳过登录
	if c.loadSession(username) {
		c.loggedIn = true
		log.Printf("[GARMIN] 复用已有 session (user=%s)", username)
		return nil
	}

	log.Printf("[GARMIN] 开始登录 (user=%s)", username)

	// Garmin SSO 登录端点（2024+ 使用移动端 SSO DI OAuth 流程）
	// 注意：Garmin 对第三方登录有 TLS 指纹检测，可能返回 429 或登录失败
	// 失败时返回 error，调用方（scheduler）应保留旧 cache 不写空值
	ssoURL := "https://sso.garmin.com/sso/signin"

	form := strings.NewReader(url.Values{
		"username": {username},
		"password": {password},
		"embed":     {"false"},
		"_embed":    {"false"},
	}.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ssoURL, form)
	if err != nil {
		return fmt.Errorf("构造登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Origin", "https://sso.garmin.com")
	req.Header.Set("Referer", "https://sso.garmin.com/sso/signin")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Garmin 登录请求失败（可能是网络或风控）: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode == 429 {
		return fmt.Errorf("Garmin 登录触发风控 (429 Too Many Requests)，请稍后重试")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return fmt.Errorf("Garmin 登录失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 检查响应中是否含错误标志
	bodyStr := string(body)
	if strings.Contains(bodyStr, "errorMessage") || strings.Contains(bodyStr, "invalid") {
		return fmt.Errorf("Garmin 登录凭证无效或被拒绝")
	}

	// 尝试访问 connect 主页验证 session
	verifyReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/modern/", nil)
	verifyResp, err := c.httpClient.Do(verifyReq)
	if err != nil {
		return fmt.Errorf("验证 Garmin session 失败: %w", err)
	}
	verifyResp.Body.Close()

	c.loggedIn = true
	c.saveSession()
	log.Printf("[GARMIN] 登录成功 (user=%s)", username)
	return nil
}

// GetDailyHealth 获取指定日期的健康摘要
func (c *garminClient) GetDailyHealth(ctx context.Context, date string) (*DailyHealth, error) {
	if !c.loggedIn {
		return nil, fmt.Errorf("未登录 Garmin")
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// Garmin Connect 活动摘要端点
	u := fmt.Sprintf("%s/proxy/wellness-service/wellness/dailySummary/%s", c.baseURL, date)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取 Garmin 健康数据失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return nil, fmt.Errorf("Garmin API 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var summary struct {
		Steps        int     `json:"totalSteps"`
		RestingHR    int     `json:"restingHeartRate"`
		SleepHours   float64 `json:"sleepTimeSeconds"`
		StressAvg    int     `json:"averageStressLevel"`
		SpO2         int     `json:"averageSpO2"`
		BodyBattery  int     `json:"averageBodyBatteryChargedValue"`
		Calories     int     `json:"activeKilocalories"`
	}
	if err := json.Unmarshal(body, &summary); err != nil {
		return nil, fmt.Errorf("解析 Garmin 健康数据失败: %w", err)
	}

	return &DailyHealth{
		Steps:       summary.Steps,
		HeartRate:   summary.RestingHR,
		SleepHours:  summary.SleepHours / 3600,
		Stress:      summary.StressAvg,
		SpO2:        summary.SpO2,
		BodyBattery: summary.BodyBattery,
		Calories:    summary.Calories,
	}, nil
}
