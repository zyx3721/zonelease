package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"zonelease/backend/api/router"
	"zonelease/backend/config"
	_ "zonelease/backend/docs"
	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/repository"
	authsvc "zonelease/backend/internal/service/auth"
	"zonelease/backend/internal/service/notify"
	"zonelease/backend/internal/service/realtime"
	syncsvc "zonelease/backend/internal/service/sync"
	"zonelease/backend/pkg/database"
)

// @title ZoneLease API
// @version 1.0
// @description ZoneLease Windows DNS / DHCP 统一管理控制台后端 API，提供认证、服务器登记、DNS/DHCP 资源管理、刷新事件和审计查询接口。
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loadEnv(logger)

	cfg, err := config.Load(logger)
	if err != nil {
		logger.Error("Load configuration failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg)
	if err != nil {
		logger.Error("Connect PostgreSQL failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool, logger); err != nil {
		logger.Error("Migrate PostgreSQL failed", "error", err)
		os.Exit(1)
	}

	redisClient, err := realtime.Connect(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		logger.Error("Connect Redis failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	store := repository.New(pool)
	if err := store.EnsureDefaultAdmin(ctx); err != nil {
		logger.Error("Initialize default admin failed", "error", err)
		os.Exit(1)
	}

	realtimeService := realtime.NewWithStream(redisClient, cfg.Runtime.RefreshTTL, cfg.Runtime.MetricStreamMaxLen)
	agentClient := agent.NewClient()
	syncService := syncsvc.New(store, agentClient, realtimeService, logger, cfg.Runtime)
	syncService.StartScheduledFullRefresh(ctx)
	syncService.StartScheduledHealthCheck(ctx)
	syncService.StartLogRetention(ctx)
	authService := authsvc.New(store, authsvc.Config{
		SessionSecret:         cfg.Auth.SessionSecret,
		SessionTTL:            cfg.Auth.SessionTTL(),
		SessionIdleTTL:        cfg.Auth.SessionIdleTTL(),
		ResetCodeTTL:          cfg.Auth.ResetCodeTTL,
		ResetCaptchaTTL:       cfg.Auth.ResetCaptchaTTL,
		ResetVerificationTTL:  cfg.Auth.ResetVerificationTTL,
		ResetSendCooldownSecs: cfg.Auth.ResetSendCooldownSecs,
		ResetRateLimitSpan:    5 * time.Minute,
		ResetRateLimitMax:     5,
	})
	authService.SetNotifier(notify.New(store))

	server := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           router.New(cfg, store, authService, realtimeService, syncService, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("ZoneLease backend listening", "addr", cfg.Server.Addr())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("ZoneLease backend stopped")
}

func loadEnv(logger *slog.Logger) {
	cwd, err := os.Getwd()
	if err == nil {
		_ = godotenv.Load(filepath.Join(cwd, ".env"))
	}
	exe, err := os.Executable()
	if err == nil {
		if err := godotenv.Load(filepath.Join(filepath.Dir(exe), ".env")); err == nil {
			logger.Info("Loaded .env beside executable")
		}
	}
}
