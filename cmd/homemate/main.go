package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/homemate/server/internal/config"
	wechatHandler "github.com/homemate/server/internal/handler/wechat"
	"github.com/homemate/server/internal/router"
	"github.com/homemate/server/internal/service/garmin"
	"github.com/homemate/server/internal/service/scheduler"
	"github.com/homemate/server/internal/service/weather"
	"github.com/homemate/server/internal/store"
)

func main() {
	// 命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg := config.Load()
	_ = configPath // 配置文件路径可通过后续扩展使用

	// 初始化数据库
	db, err := store.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("[FATAL] 数据库初始化失败: %v", err)
	}
	defer db.Close()

	// 构造 Garmin 客户端（从 config 读取默认凭证，按成员配置可覆盖）
	var garminClient garmin.Client
	if cfg.Garmin.Username != "" || cfg.Garmin.BaseURL != "" {
		garminClient = garmin.NewClient(cfg.Garmin.TokenDir, cfg.Garmin.BaseURL)
		log.Printf("[INFO] Garmin 客户端已初始化 (tokenDir=%s)", cfg.Garmin.TokenDir)
	} else {
		log.Println("[INFO] Garmin 未配置，相关同步将跳过")
	}

	// 构造 AMAP 天气客户端
	var weatherClient weather.Client
	if cfg.Amap.APIKey != "" {
		weatherClient = weather.NewClient(cfg.Amap.APIKey, cfg.Amap.BaseURL)
		log.Printf("[INFO] AMAP 天气客户端已初始化 (city=%s)", cfg.Amap.City)
	} else {
		log.Println("[INFO] AMAP 未配置，天气同步将跳过")
	}

	// 注入 WeCom 配置并构造推送器
	wechatHandler.SetWeComConfig(cfg.WeCom.WebhookURL, cfg.WeCom.EnablePush)
	pusher := wechatHandler.GetPusher()
	if cfg.WeCom.EnablePush {
		log.Printf("[INFO] WeCom 推送已启用 (webhook 配置)")
	} else {
		log.Println("[INFO] WeCom 推送未启用，使用 NoopPusher")
	}

	// 初始化定时任务调度器
	sched := scheduler.New(db, garminClient, weatherClient, pusher)
	schedCfg := scheduler.DefaultTaskConfig()
	// TODO: 从配置文件读取调度器配置
	sched.Start(schedCfg)
	log.Println("[INFO] 调度器已启动")

	// 设置路由
	r := router.Setup(db, sched, cfg.JWT.Secret, cfg.Server.Mode)

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  cfg.Server.Timeout.Read,
		WriteTimeout: cfg.Server.Timeout.Write,
		IdleTimeout:  cfg.Server.Timeout.Idle,
	}

	// 优雅关闭
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		log.Printf("[INFO] 收到信号 %v，开始优雅关闭...", sig)

		// 停止调度器
		sched.Stop()

		// 关闭 HTTP 服务器
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("[ERROR] 服务器关闭出错: %v", err)
		}
		log.Println("[INFO] 服务器已关闭")
	}()

	// 启动服务器
	log.Printf("[INFO] HomeMate v3.1 启动于 :%s (mode=%s)", cfg.Server.Port, cfg.Server.Mode)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[FATAL] 服务器启动失败: %v", err)
	}
}