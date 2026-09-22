// zonelease 后端服务入口：解析命令行参数、加载配置并启动 HTTP 服务
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"zonelease/backend/api/router"
	"zonelease/backend/config"
	_ "zonelease/backend/docs"
	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/buildinfo"
	"zonelease/backend/internal/repository"
	authsvc "zonelease/backend/internal/service/auth"
	"zonelease/backend/internal/service/notify"
	"zonelease/backend/internal/service/realtime"
	syncsvc "zonelease/backend/internal/service/sync"
	"zonelease/backend/pkg/database"
)

// flagValues 汇总命令行参数的显式取值，零值表示未设置
type flagValues struct {
	envPath             string
	host                string
	port                string
	mode                string
	dbHost              string
	dbPort              string
	dbName              string
	dbUser              string
	dbPassword          string
	dbSSLMode           string
	redisAddr           string
	redisPassword       string
	redisDB             int
	jwtSecret           string
	sessionTTLHours     int
	dnsSyncInterval     string
	dhcpSyncInterval    string
	metricRetentionDays int
	logRetentionDays    int
	metricStreamMaxLen  int
	corsOrigin          string
}

// versionText 组装 -v/-version 输出的版本信息文本
func versionText() string {
	return fmt.Sprintf("zonelease %s\ncommit: %s\nbuild: %s\ngo: %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate, runtime.Version())
}

// setUsage 自定义 -h/--help 输出：-v 与 -version 合并一行，各参数描述统一换行缩进对齐
func setUsage() {
	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintf(w, "Usage of %s:\n", os.Args[0])
		flag.VisitAll(func(f *flag.Flag) {
			if f.Name == "version" {
				return
			}
			if f.Name == "v" {
				fmt.Fprintf(w, "  -v, -version\n    \t%s\n", f.Usage)
				return
			}
			name, usage := flag.UnquoteUsage(f)
			fmt.Fprintf(w, "  -%s %s\n    \t%s\n", f.Name, name, usage)
		})
	}
}

// overridesFromFlags 将命令行参数显式值映射为配置加载的覆盖键值
func overridesFromFlags(v flagValues) map[string]string {
	overrides := make(map[string]string)
	if v.envPath != "" {
		overrides["env"] = v.envPath
	}
	if v.host != "" {
		overrides["server_host"] = v.host
	}
	if v.port != "" {
		overrides["server_port"] = v.port
	}
	if v.mode != "" {
		overrides["server_mode"] = v.mode
	}
	if v.dbHost != "" {
		overrides["db_host"] = v.dbHost
	}
	if v.dbPort != "" {
		overrides["db_port"] = v.dbPort
	}
	if v.dbName != "" {
		overrides["db_name"] = v.dbName
	}
	if v.dbUser != "" {
		overrides["db_user"] = v.dbUser
	}
	if v.dbPassword != "" {
		overrides["db_password"] = v.dbPassword
	}
	if v.dbSSLMode != "" {
		overrides["db_sslmode"] = v.dbSSLMode
	}
	if v.redisAddr != "" {
		overrides["redis_addr"] = v.redisAddr
	}
	if v.redisPassword != "" {
		overrides["redis_password"] = v.redisPassword
	}
	if v.redisDB >= 0 {
		overrides["redis_db"] = strconv.Itoa(v.redisDB)
	}
	if v.jwtSecret != "" {
		overrides["jwt_secret"] = v.jwtSecret
	}
	if v.sessionTTLHours > 0 {
		overrides["jwt_expire_hours"] = strconv.Itoa(v.sessionTTLHours)
	}
	if v.dnsSyncInterval != "" {
		overrides["runtime_dns_deep_sync_interval"] = v.dnsSyncInterval
	}
	if v.dhcpSyncInterval != "" {
		overrides["runtime_dhcp_deep_sync_interval"] = v.dhcpSyncInterval
	}
	if v.metricRetentionDays > 0 {
		overrides["metric_retention_days"] = strconv.Itoa(v.metricRetentionDays)
	}
	if v.logRetentionDays > 0 {
		overrides["log_retention_days"] = strconv.Itoa(v.logRetentionDays)
	}
	if v.metricStreamMaxLen > 0 {
		overrides["metric_stream_maxlen"] = strconv.Itoa(v.metricStreamMaxLen)
	}
	if v.corsOrigin != "" {
		overrides["cors_origin"] = v.corsOrigin
	}
	return overrides
}

// @title ZoneLease API
// @version 1.0
// @description ZoneLease Windows DNS / DHCP 统一管理控制台后端 API，提供认证、服务器登记、DNS/DHCP 资源管理、刷新事件和审计查询接口。
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	showVersion := flag.Bool("v", false, "显示版本信息并退出")
	flag.BoolVar(showVersion, "version", false, "显示版本信息并退出")
	envPath := flag.String("env", "", ".env 配置文件路径，默认按当前目录、可执行文件同目录顺序查找")
	host := flag.String("host", "", "后端监听地址，等价环境变量 SERVER_HOST")
	port := flag.String("port", "", "后端监听端口，等价环境变量 SERVER_PORT")
	mode := flag.String("mode", "", "运行模式 release 或 debug，等价环境变量 SERVER_MODE")
	dbHost := flag.String("db-host", "", "PostgreSQL 主机，等价环境变量 DB_HOST")
	dbPort := flag.String("db-port", "", "PostgreSQL 端口，等价环境变量 DB_PORT")
	dbName := flag.String("db-name", "", "PostgreSQL 数据库名，等价环境变量 DB_NAME")
	dbUser := flag.String("db-user", "", "PostgreSQL 用户，等价环境变量 DB_USER")
	dbPassword := flag.String("db-password", "", "PostgreSQL 密码，等价环境变量 DB_PASSWORD")
	dbSSLMode := flag.String("db-sslmode", "", "PostgreSQL SSL 模式，等价环境变量 DB_SSLMODE")
	redisAddr := flag.String("redis-addr", "", "Redis 地址，等价环境变量 REDIS_ADDR")
	redisPassword := flag.String("redis-password", "", "Redis 密码，等价环境变量 REDIS_PASSWORD")
	redisDB := flag.Int("redis-db", -1, "Redis 库编号，等价环境变量 REDIS_DB")
	jwtSecret := flag.String("jwt-secret", "", "会话令牌签名密钥，等价环境变量 JWT_SECRET")
	sessionTTLHours := flag.Int("session-ttl", 0, "登录会话有效期（小时），等价环境变量 JWT_EXPIRE_HOURS")
	dnsSyncInterval := flag.String("dns-sync-interval", "", "DNS 深度同步间隔（如 1h、1d），等价环境变量 RUNTIME_DNS_DEEP_SYNC_INTERVAL")
	dhcpSyncInterval := flag.String("dhcp-sync-interval", "", "DHCP 深度同步间隔（如 1h、1d），等价环境变量 RUNTIME_DHCP_DEEP_SYNC_INTERVAL")
	metricRetentionDays := flag.Int("metric-retention-days", 0, "指标数据保留天数，等价环境变量 METRIC_RETENTION_DAYS")
	logRetentionDays := flag.Int("log-retention-days", 0, "日志数据保留天数，等价环境变量 LOG_RETENTION_DAYS")
	metricStreamMaxLen := flag.Int("metric-stream-maxlen", 0, "Redis 指标流最大长度，等价环境变量 METRIC_STREAM_MAXLEN")
	corsOrigin := flag.String("cors-origin", "", "允许的跨域来源，等价环境变量 CORS_ORIGIN")
	setUsage()
	flag.Parse()

	if *showVersion {
		fmt.Print(versionText())
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loadEnv(logger, *envPath)

	cfg, err := config.LoadWithOverrides(logger, overridesFromFlags(flagValues{
		envPath:             *envPath,
		host:                *host,
		port:                *port,
		mode:                *mode,
		dbHost:              *dbHost,
		dbPort:              *dbPort,
		dbName:              *dbName,
		dbUser:              *dbUser,
		dbPassword:          *dbPassword,
		dbSSLMode:           *dbSSLMode,
		redisAddr:           *redisAddr,
		redisPassword:       *redisPassword,
		redisDB:             *redisDB,
		jwtSecret:           *jwtSecret,
		sessionTTLHours:     *sessionTTLHours,
		dnsSyncInterval:     *dnsSyncInterval,
		dhcpSyncInterval:    *dhcpSyncInterval,
		metricRetentionDays: *metricRetentionDays,
		logRetentionDays:    *logRetentionDays,
		metricStreamMaxLen:  *metricStreamMaxLen,
		corsOrigin:          *corsOrigin,
	}))
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
		logger.Info("ZoneLease backend listening", "addr", cfg.Server.Addr(), "version", buildinfo.Version)
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

// loadEnv 加载 .env 配置文件，显式指定路径时仅加载该路径，否则按当前目录、可执行文件同目录顺序查找
func loadEnv(logger *slog.Logger, explicitPath string) {
	if explicitPath != "" {
		if err := godotenv.Load(explicitPath); err != nil {
			logger.Warn("Load .env from command line argument failed", "path", explicitPath, "error", err)
			return
		}
		logger.Info("Loaded .env from command line argument", "path", explicitPath)
		return
	}
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
