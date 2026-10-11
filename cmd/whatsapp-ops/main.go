package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"whatsapp-ops/internal/api"
	"whatsapp-ops/internal/broadcast"
	"whatsapp-ops/internal/config"
	"whatsapp-ops/internal/database"
	"whatsapp-ops/internal/risk"
	"whatsapp-ops/internal/whatsapp"
	"go.uber.org/zap"
)

func main() {
	// 初始化日志
	if err := initLogger(); err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer zap.L().Sync()

	// 加载配置
	if err := config.Load(); err != nil {
		zap.L().Fatal("加载配置失败", zap.Error(err))
	}

	cfg := config.Get()
	zap.L().Info("启动 WhatsApp Ops",
		zap.String("version", cfg.App.Version),
		zap.String("data_dir", cfg.App.DataDir),
		zap.Int("port", cfg.Server.Port))

	// 初始化数据库
	if err := database.Init(); err != nil {
		zap.L().Fatal("初始化数据库失败", zap.Error(err))
	}
	defer database.CloseAll()

	// 初始化风控引擎
	if cfg.Risk.Enabled {
		if err := risk.Init(); err != nil {
			zap.L().Warn("风控引擎初始化失败", zap.Error(err))
		} else {
			risk.LoadExampleWords()
		}
	}

	// 初始化 WhatsApp 管理器
	waMgr := whatsapp.NewManager()
	_ = waMgr.LoadAccounts()

	// 初始化群发调度器
	var bcScheduler *broadcast.Scheduler
	if cfg.Broadcast.Enabled {
		bcScheduler = broadcast.NewScheduler(waMgr, risk.GetEngine(), zap.L())
		bcScheduler.Start()
		zap.L().Info("群发调度器已启动")
	}

	// 启动 HTTP 服务器
	router := api.NewRouter(waMgr)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		zap.L().Info("HTTP 服务启动", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("HTTP 服务错误", zap.Error(err))
		}
	}()

	// 自动打开浏览器
	if cfg.App.AutoOpenBrowser && cfg.Server.Host != "0.0.0.0" {
		url := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
		go openBrowser(url)
	}

	// 优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	zap.L().Info("正在关闭服务...")

	// 停止群发调度器
	if bcScheduler != nil {
		bcScheduler.Stop()
		zap.L().Info("群发调度器已停止")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Error("服务器关闭失败", zap.Error(err))
	}
	zap.L().Info("服务已关闭")
}

func initLogger() error {
	cfg := config.Get()
	if cfg == nil {
		// 配置未加载时使用默认值
		zapConfig := zap.NewDevelopmentConfig()
		zapConfig.OutputPaths = []string{"stdout"}
		zapConfig.ErrorOutputPaths = []string{"stdout"}
		logger, err := zapConfig.Build()
		if err != nil {
			return err
		}
		zap.ReplaceGlobals(logger)
		return nil
	}

	logPath := filepath.Join(cfg.Log.Dir, fmt.Sprintf("whatsapp-ops-%s.log", time.Now().Format("20060102")))
	// 实际项目中使用 zap Lumberjack 进行日志轮转
	zapConfig := zap.NewProductionConfig()
	zapConfig.OutputPaths = []string{"stdout", logPath}
	zapConfig.ErrorOutputPaths = []string{"stdout", logPath}
	zapConfig.Level = zap.NewAtomicLevelAt(parseLogLevel(cfg.Log.Level))

	logger, err := zapConfig.Build()
	if err != nil {
		return err
	}
	zap.ReplaceGlobals(logger)
	return nil
}

func parseLogLevel(level string) zap.AtomicLevel {
	switch level {
	case "debug":
		return zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn", "warning":
		return zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		return zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		return zap.NewAtomicLevelAt(zap.InfoLevel)
	}
}

func openBrowser(url string) {
	// 使用 Go 执行系统命令打开浏览器
	// Windows: start url ; Linux/macOS: xdg-open/open
	time.Sleep(500 * time.Millisecond)
	os.StartProcess(filepath.Join(os.Getenv("COMSPEC"), "/c", "start", url))
}