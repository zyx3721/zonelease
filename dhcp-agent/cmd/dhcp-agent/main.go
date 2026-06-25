package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"zonelease/dhcp-agent/internal/config"
	"zonelease/dhcp-agent/internal/dhcp"
	"zonelease/dhcp-agent/internal/server"
)

func main() {
	bootstrapLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loadDotEnv(bootstrapLogger)
	cfg := config.Load()
	logFile, logger, err := newLogger(cfg.LogPath)
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to initialize dhcp agent log: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer logFile.Close()
	slog.SetDefault(logger)

	app := server.New(cfg, dhcp.NewPowerShellProviderWithTimeout(time.Duration(cfg.PowerShellTimeoutSeconds)*time.Second), logger)
	httpServer := &http.Server{Addr: cfg.Addr(), Handler: app.Routes(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		logger.Info("dhcp agent listening", "addr", cfg.Addr(), "logPath", cfg.LogPath, "powershellTimeoutSeconds", cfg.PowerShellTimeoutSeconds)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("dhcp agent stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("dhcp agent shutdown failed", "error", err)
		os.Exit(1)
	}
}

func newLogger(path string) (*os.File, *slog.Logger, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return file, slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})), nil
}

func loadDotEnv(logger *slog.Logger) {
	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), ".env")
		if err := godotenv.Load(path); err == nil {
			logger.Info("loaded .env", "path", path)
			return
		}
	}
	if err := godotenv.Load(); err == nil {
		logger.Info("loaded .env from working directory")
	}
}
