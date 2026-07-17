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
	"github.com/homemate/server/internal/router"
	"github.com/homemate/server/internal/service/scheduler"
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

	// 初始化定时任务调度器
	sched := scheduler.New(db)
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
	log.Printf("[INFO] HomeMate v3.0 启动于 :%s (mode=%s)", cfg.Server.Port, cfg.Server.Mode)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[FATAL] 服务器启动失败: %v", err)
	}
}