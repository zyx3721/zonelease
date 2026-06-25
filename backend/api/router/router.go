package router

import (
	"log/slog"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"zonelease/backend/config"
	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/repository"
	"zonelease/backend/internal/service/auth"
	"zonelease/backend/internal/service/realtime"
	syncsvc "zonelease/backend/internal/service/sync"
)

type Router struct {
	cfg      config.Config
	store    *repository.Store
	logger   *slog.Logger
	agent    *agent.Client
	auth     *auth.Service
	realtime *realtime.Service
	sync     *syncsvc.Service
	refresh  *operationRefreshScheduler
}

func New(cfg config.Config, store *repository.Store, authService *auth.Service, realtimeService *realtime.Service, syncService *syncsvc.Service, logger *slog.Logger) http.Handler {
	r := &Router{
		cfg:      cfg,
		store:    store,
		logger:   logger,
		agent:    agent.NewClient(),
		auth:     authService,
		realtime: realtimeService,
		sync:     syncService,
	}
	r.refresh = newOperationRefreshScheduler(r)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", r.health)
	mux.HandleFunc("GET /swagger/", httpSwagger.WrapHandler)
	mux.HandleFunc("POST /api/auth/login", r.login)
	mux.HandleFunc("GET /api/auth/providers", r.publicAuthProviders)
	mux.HandleFunc("POST /api/auth/logout", r.logout)
	mux.HandleFunc("GET /api/auth/me", r.me)
	mux.HandleFunc("GET /api/public/base", r.publicSystemBaseConfig)
	mux.HandleFunc("GET /api/auth/password-reset/captcha", r.passwordResetCaptcha)
	mux.HandleFunc("POST /api/auth/password-reset/verify", r.passwordResetVerify)
	mux.HandleFunc("POST /api/auth/password-reset/send", r.passwordResetSend)
	mux.HandleFunc("POST /api/auth/password-reset/confirm", r.passwordResetConfirm)
	mux.HandleFunc("GET /api/events", r.events)

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/auth/change-password", r.changePassword)
	protected.HandleFunc("GET /api/state", r.state)
	protected.HandleFunc("GET /api/notifications", r.notifications)
	protected.HandleFunc("GET /api/notifications/unread-count", r.unreadNotificationCount)
	protected.HandleFunc("POST /api/notifications/read-all", r.markAllNotificationsRead)
	protected.HandleFunc("POST /api/notifications/clear", r.clearNotifications)
	protected.HandleFunc("POST /api/notifications/", r.markNotificationRead)
	protected.HandleFunc("POST /api/refresh", r.createRefresh)
	protected.HandleFunc("GET /api/refresh/tasks", r.refreshTasks)
	protected.HandleFunc("POST /api/servers", r.createServer)
	protected.HandleFunc("POST /api/servers/probe", r.probeServer)
	protected.HandleFunc("DELETE /api/servers/", r.deleteServer)
	protected.HandleFunc("POST /api/servers/", r.serverAction)
	protected.HandleFunc("POST /api/dns/zones", r.createZone)
	protected.HandleFunc("POST /api/dns/zones/", r.zoneAction)
	protected.HandleFunc("DELETE /api/dns/zones/", r.deleteZone)
	protected.HandleFunc("POST /api/dns/records", r.createRecord)
	protected.HandleFunc("PUT /api/dns/records/", r.updateRecord)
	protected.HandleFunc("DELETE /api/dns/records/", r.deleteRecord)
	protected.HandleFunc("POST /api/dhcp/scopes", r.createScope)
	protected.HandleFunc("PUT /api/dhcp/scopes/", r.updateScope)
	protected.HandleFunc("POST /api/dhcp/scopes/", r.scopeAction)
	protected.HandleFunc("DELETE /api/dhcp/scopes/", r.deleteScope)
	protected.HandleFunc("POST /api/dhcp/exclusions", r.createExclusion)
	protected.HandleFunc("DELETE /api/dhcp/exclusions/", r.deleteExclusion)
	protected.HandleFunc("DELETE /api/dhcp/leases/", r.deleteLease)
	protected.HandleFunc("POST /api/dhcp/reservations", r.createReservation)
	protected.HandleFunc("PUT /api/dhcp/reservations/", r.updateReservation)
	protected.HandleFunc("DELETE /api/dhcp/reservations/", r.deleteReservation)
	mux.Handle("/api/state", r.requireAuth(protected))
	mux.Handle("/api/notifications", r.requireAuth(protected))
	mux.Handle("/api/notifications/", r.requireAuth(protected))
	mux.Handle("/api/auth/change-password", r.requireAuth(protected))
	mux.Handle("/api/refresh", r.requireAuth(protected))
	mux.Handle("/api/refresh/", r.requireAuth(protected))
	mux.Handle("/api/servers", r.requireAuth(protected))
	mux.Handle("/api/servers/", r.requireAuth(protected))
	mux.Handle("GET /api/settings/base", r.requirePermission("settings.base.read", http.HandlerFunc(r.getSystemBaseConfig)))
	mux.Handle("PUT /api/settings/base", r.requirePermission("settings.base.manage", http.HandlerFunc(r.updateSystemBaseConfig)))
	mux.Handle("GET /api/settings/users", r.requirePermission("settings.users.read", http.HandlerFunc(r.listManagedUsers)))
	mux.Handle("POST /api/settings/users", r.requirePermission("settings.users.manage", http.HandlerFunc(r.createManagedUser)))
	mux.Handle("PUT /api/settings/users/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.managedUserRoute)))
	mux.Handle("DELETE /api/settings/users/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.managedUserRoute)))
	mux.Handle("POST /api/settings/users/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.managedUserRoute)))
	mux.Handle("GET /api/settings/user-groups", r.requirePermission("settings.users.read", http.HandlerFunc(r.listUserGroups)))
	mux.Handle("POST /api/settings/user-groups", r.requirePermission("settings.users.manage", http.HandlerFunc(r.createUserGroup)))
	mux.Handle("PUT /api/settings/user-groups/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.userGroupRoute)))
	mux.Handle("DELETE /api/settings/user-groups/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.userGroupRoute)))
	mux.Handle("GET /api/settings/roles", r.requirePermission("settings.users.read", http.HandlerFunc(r.listRoles)))
	mux.Handle("POST /api/settings/roles", r.requirePermission("settings.users.manage", http.HandlerFunc(r.createRole)))
	mux.Handle("PUT /api/settings/roles/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.roleRoute)))
	mux.Handle("DELETE /api/settings/roles/", r.requirePermission("settings.users.manage", http.HandlerFunc(r.roleRoute)))
	mux.Handle("GET /api/settings/permissions", r.requirePermission("settings.users.read", http.HandlerFunc(r.listPermissions)))
	mux.Handle("GET /api/settings/auth-providers", r.requirePermission("settings.auth.read", http.HandlerFunc(r.listAuthProviders)))
	mux.Handle("PUT /api/settings/auth-providers/", r.requirePermission("settings.auth.manage", http.HandlerFunc(r.authProviderRoute)))
	mux.Handle("POST /api/settings/auth-providers/", r.requirePermission("settings.auth.manage", http.HandlerFunc(r.authProviderRoute)))
	mux.Handle("GET /api/settings/notifications", r.requirePermission("settings.notifications.read", http.HandlerFunc(r.listNotificationChannels)))
	mux.Handle("PUT /api/settings/notifications/", r.requirePermission("settings.notifications.manage", http.HandlerFunc(r.notificationRoute)))
	mux.Handle("POST /api/settings/notifications/", r.requirePermission("settings.notifications.manage", http.HandlerFunc(r.notificationRoute)))
	mux.Handle("/api/dns/", r.requireAuth(protected))
	mux.Handle("/api/dhcp/", r.requireAuth(protected))

	return r.withCORS(r.withLog(mux))
}
