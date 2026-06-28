package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	stdsync "sync"
	"time"

	"zonelease/backend/config"
	"zonelease/backend/internal/agent"
	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
	"zonelease/backend/internal/service/realtime"
	"zonelease/backend/internal/strutil"
)

const (
	RefreshAllType       = "runtime.refresh.all"
	RefreshDNSAllType    = "runtime.refresh.dns.all"
	RefreshDHCPAllType   = "runtime.refresh.dhcp.all"
	RefreshServerType    = "runtime.refresh.server"
	RefreshDNSZoneType   = "runtime.refresh.dns.zone"
	RefreshDHCPScopeType = "runtime.refresh.dhcp.scope"

	notificationSourceAgentHealth = "agent_health"

	agentHealthCheckLockKey     = "zonelease:lock:agent-health-check"
	scheduledDNSRefreshLockKey  = "zonelease:lock:refresh:scheduled-dns"
	scheduledDHCPRefreshLockKey = "zonelease:lock:refresh:scheduled-dhcp"
	refreshTaskRuntimeTTL       = 30 * time.Minute

	unreadNotificationCountCacheKey = "zonelease:runtime:notifications:unread-count"
)

var errAgentSyncRunning = errors.New("当前 Agent 正在同步，请稍后再操作")

type Service struct {
	store    *repository.Store
	agent    *agent.Client
	realtime *realtime.Service
	logger   *slog.Logger
	cfg      config.RuntimeConfig
}

type runtimeLimits struct {
	SyncConcurrency          int
	DNSRecordConcurrency     int
	DHCPScopeConcurrency     int
	AgentOfflineFailureCount int
	AgentHealthConcurrency   int
	AgentOperationTimeout    time.Duration
	AgentFullSyncTimeout     time.Duration
}

type ZoneTarget struct {
	ServerID   string `json:"serverId"`
	ServerName string `json:"serverName,omitempty"`
	ZoneID     string `json:"zoneId"`
	ZoneName   string `json:"zoneName"`
}

type ServerTarget struct {
	ServerID   string `json:"serverId"`
	ServerName string `json:"serverName,omitempty"`
}

type DHCPScopeTarget struct {
	ServerID        string `json:"serverId"`
	ServerName      string `json:"serverName,omitempty"`
	ScopeID         string `json:"scopeId,omitempty"`
	ScopeExternalID string `json:"-"`
	ScopeName       string `json:"scopeName,omitempty"`
}

type refreshProgressPayload struct {
	Message       string               `json:"message"`
	Warn          string               `json:"warn,omitempty"`
	StartedAt     string               `json:"startedAt,omitempty"`
	FinishedAt    string               `json:"finishedAt,omitempty"`
	TotalAgents   int                  `json:"totalAgents"`
	StartedAgents int                  `json:"startedAgents"`
	SyncedAgents  int                  `json:"syncedAgents"`
	SkippedAgents int                  `json:"skippedAgents"`
	FailedAgents  int                  `json:"failedAgents"`
	ResourceType  string               `json:"resourceType,omitempty"`
	ResourceID    string               `json:"resourceId,omitempty"`
	ResourceName  string               `json:"resourceName,omitempty"`
	CurrentAgent  string               `json:"currentAgent,omitempty"`
	AgentResults  []refreshAgentResult `json:"agentResults"`
	AgentEvent    *refreshAgentResult  `json:"agentEvent,omitempty"`
	Error         string               `json:"error,omitempty"`
}

type refreshAgentResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Warn   string `json:"warn,omitempty"`
}

func New(store *repository.Store, agentClient *agent.Client, realtimeService *realtime.Service, logger *slog.Logger, cfg config.RuntimeConfig) *Service {
	return &Service{store: store, agent: agentClient, realtime: realtimeService, logger: logger, cfg: cfg}
}

func (s *Service) StartScheduledFullRefresh(ctx context.Context) {
	s.startScheduledRoleRefresh(ctx, RefreshDNSAllType, scheduledDNSRefreshLockKey, s.cfg.DNSDeepSyncInterval, "DNS 定时全量刷新已排队")
	s.startScheduledRoleRefresh(ctx, RefreshDHCPAllType, scheduledDHCPRefreshLockKey, s.cfg.DHCPDeepSyncInterval, "DHCP 定时全量刷新已排队")
}

func (s *Service) startScheduledRoleRefresh(ctx context.Context, taskType, lockKey string, interval time.Duration, queuedMessage string) {
	if interval <= 0 {
		s.logger.Info("Scheduled role refresh disabled", "type", taskType)
		return
	}
	go func() {
		s.logger.Info("Scheduled role refresh started", "type", taskType, "interval", interval.String())
		for {
			wait := nextScheduledRoleRefreshDelay(time.Now(), interval)
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				lock, locked, err := s.realtime.TryLock(ctx, lockKey, s.refreshTaskLockTTL(ctx))
				if err != nil {
					s.logger.Warn("Acquire scheduled role refresh lock failed", "type", taskType, "error", err)
				} else if !locked {
					s.logger.Info("Skip scheduled role refresh because another instance holds the lock", "type", taskType)
					continue
				}
				task, err := s.store.CreateRefreshTask(ctx, taskType, map[string]any{"message": queuedMessage}, "")
				if err != nil {
					if locked {
						_ = s.realtime.Unlock(context.Background(), lock)
					}
					s.logger.Warn("Create scheduled role refresh task failed", "type", taskType, "error", err)
					continue
				}
				_ = s.publish(context.Background(), taskType, task.ID, "queued", queuedMessage)
				go func() {
					defer func() {
						if locked {
							_ = s.realtime.Unlock(context.Background(), lock)
						}
					}()
					s.RunRefreshTask(context.Background(), task.ID, taskType, nil)
				}()
			}
		}
	}()
}

func nextScheduledRoleRefreshDelay(now time.Time, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	seconds := int64(interval.Seconds())
	if seconds <= 0 {
		return interval
	}
	daySeconds := int64((24 * time.Hour) / time.Second)
	var elapsed int64
	if seconds%daySeconds == 0 {
		year, month, day := now.Date()
		midnight := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
		elapsed = int64(now.Sub(midnight).Seconds())
	} else if daySeconds%seconds == 0 {
		year, month, day := now.Date()
		midnight := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
		elapsed = int64(now.Sub(midnight).Seconds())
	} else {
		elapsed = now.Unix()
	}
	remaining := seconds - elapsed%seconds
	if remaining <= 0 {
		remaining = seconds
	}
	return time.Duration(remaining) * time.Second
}

func (s *Service) StartScheduledHealthCheck(ctx context.Context) {
	go func() {
		s.logger.Info("Scheduled agent health check started")
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		nextRun := time.Time{}
		for {
			now := time.Now()
			interval := s.healthCheckInterval(context.Background())
			if nextRun.IsZero() || !now.Before(nextRun) {
				nextRun = now.Add(interval)
				s.checkAllServerHealth(context.Background())
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) checkAllServerHealth(ctx context.Context) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		s.logger.Warn("List servers for health check failed", "error", err)
		return
	}
	limits := s.runtimeLimits(ctx)
	targets := make([]domain.Server, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server.AgentURL) == "" {
			continue
		}
		if s.isAgentSyncRunning(ctx, server.ID) {
			s.logger.Info("Skip agent health check because sync is running", "server", server.ID)
			continue
		}
		targets = append(targets, server)
	}
	lockTTL := limits.AgentOperationTimeout*time.Duration(maxPositive(len(targets), 1)) + time.Minute
	lock, locked, err := s.realtime.TryLock(ctx, agentHealthCheckLockKey, lockTTL)
	if err != nil {
		s.logger.Warn("Acquire agent health check lock failed", "error", err)
	} else if !locked {
		s.logger.Info("Skip agent health check because another instance holds the lock")
		return
	} else {
		defer func() {
			if err := s.realtime.Unlock(context.Background(), lock); err != nil {
				s.logger.Warn("Release agent health check lock failed", "error", err)
			}
		}()
	}
	s.checkServerHealthBatch(ctx, targets, limits)
	if len(targets) > 0 {
		_ = s.publish(ctx, "runtime.updated", "", "success", "运行态已更新")
	}
}

func (s *Service) checkServerHealthBatch(ctx context.Context, servers []domain.Server, limits runtimeLimits) {
	if len(servers) == 0 {
		return
	}
	workers := minPositive(limits.AgentHealthConcurrency, len(servers))
	if workers <= 1 {
		for _, server := range servers {
			s.checkServerHealth(ctx, server, limits)
		}
		return
	}
	jobs := make(chan domain.Server)
	var wg stdsync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for server := range jobs {
				s.checkServerHealth(ctx, server, limits)
			}
		}()
	}
	for _, server := range servers {
		jobs <- server
	}
	close(jobs)
	wg.Wait()
}

func (s *Service) checkServerHealth(ctx context.Context, server domain.Server, limits runtimeLimits) {
	status := "Online"
	detail := ""
	started := time.Now()
	agentCtx, cancel := context.WithTimeout(ctx, limits.AgentOperationTimeout)
	defer cancel()
	if err := s.agent.Health(agentCtx, server.AgentURL, server.APIKey, server.TLSInsecure); err != nil {
		status = "Offline"
		detail = err.Error()
	}
	healthUpdate, err := s.store.UpdateServerHealth(ctx, server.ID, status, limits.AgentOfflineFailureCount)
	if err != nil {
		s.logger.Warn("Update server health failed", "server", server.ID, "error", err)
		return
	}
	s.cacheAgentHealth(ctx, server, status, detail, healthUpdate.FailureCount, time.Since(started))
	if healthUpdate.Status == "Online" {
		if err := s.store.DismissNotificationsBySource(ctx, notificationSourceAgentHealth, server.ID); err != nil {
			s.logger.Warn("Clear agent offline notification failed", "server", server.ID, "error", err)
		} else {
			s.invalidateUnreadNotificationCount(ctx)
		}
		return
	}
	if healthUpdate.BecameOffline() {
		s.notifyAgentOffline(ctx, server, detail)
	}
}

func (s *Service) notifyAgentOffline(ctx context.Context, server domain.Server, detail string) {
	if strings.TrimSpace(server.ID) == "" {
		return
	}
	name := strings.TrimSpace(server.Name)
	if name == "" {
		name = strings.TrimSpace(server.Host)
	}
	if name == "" {
		name = server.ID
	}
	_, _, err := s.store.CreateNotificationIfUnreadMissing(ctx, "critical", "Agent 离线", name+" 无法通过健康检查", notificationSourceAgentHealth, server.ID, map[string]any{
		"server":   name,
		"serverId": server.ID,
		"status":   "Offline",
		"error":    detail,
	})
	if err != nil {
		s.logger.Warn("Create agent offline notification failed", "server", server.ID, "error", err)
		return
	}
	s.invalidateUnreadNotificationCount(ctx)
}

func (s *Service) cacheAgentHealth(ctx context.Context, server domain.Server, status, detail string, failureCount int, duration time.Duration) {
	if strings.TrimSpace(server.ID) == "" {
		return
	}
	if err := s.realtime.Set(ctx, agentHealthRuntimeKey(server.ID), map[string]any{
		"serverId":       server.ID,
		"serverName":     server.Name,
		"status":         status,
		"error":          detail,
		"failureCount":   failureCount,
		"durationMillis": duration.Milliseconds(),
		"checkedAt":      time.Now().UTC().Format(time.RFC3339Nano),
	}, 30*time.Minute); err != nil {
		s.logger.Warn("Cache agent health runtime failed", "server", server.ID, "error", err)
	}
}

func (s *Service) RunRefreshTask(ctx context.Context, taskID, taskType string, target any) {
	lockKey := refreshTaskLockKey(taskType, target)
	var lock realtime.Lock
	if lockKey != "" {
		var locked bool
		var err error
		lock, locked, err = s.realtime.TryLock(ctx, lockKey, s.refreshTaskLockTTL(ctx))
		if err != nil {
			s.logger.Warn("Acquire refresh task lock failed", "task", taskID, "type", taskType, "lock", lockKey, "error", err)
		} else if !locked {
			message := "重复刷新任务已跳过"
			if taskType == RefreshServerType {
				message = "当前 Agent 正在同步，请稍后再操作"
			}
			if err := s.store.DeleteRefreshTask(ctx, taskID); err != nil {
				s.logger.Warn("Delete duplicate refresh task failed", "task", taskID, "type", taskType, "error", err)
			}
			_ = s.publish(ctx, taskType, taskID, "failed", message)
			return
		}
		defer func() {
			if err := s.realtime.Unlock(context.Background(), lock); err != nil {
				s.logger.Warn("Release refresh task lock failed", "task", taskID, "type", taskType, "lock", lockKey, "error", err)
			}
		}()
	}
	s.updateRefreshTask(ctx, taskID, "running", refreshTaskPayload(taskType, target, "刷新任务运行中", ""))
	_ = s.publish(ctx, taskType, taskID, "running", "刷新任务运行中")

	var err error
	var progress *refreshProgressPayload
	switch taskType {
	case RefreshServerType:
		serverTarget, ok := target.(*ServerTarget)
		if !ok || serverTarget == nil {
			err = errors.New("server target is required")
		} else {
			var server domain.Server
			server, err = s.store.GetServer(ctx, serverTarget.ServerID)
			if err == nil {
				serverTarget.ServerName = server.Name
				err = s.SyncServer(ctx, server)
			}
		}
	case RefreshDNSZoneType:
		zoneTarget, ok := target.(*ZoneTarget)
		if !ok || zoneTarget == nil {
			err = errors.New("zone target is required")
		} else {
			if server, serverErr := s.store.GetServer(ctx, zoneTarget.ServerID); serverErr == nil {
				zoneTarget.ServerName = server.Name
			}
			err = s.SyncDNSZone(ctx, zoneTarget.ServerID, zoneTarget.ZoneName)
		}
	case RefreshDHCPScopeType:
		scopeTarget, ok := target.(*DHCPScopeTarget)
		if !ok || scopeTarget == nil {
			err = errors.New("dhcp scope target is required")
		} else {
			if server, serverErr := s.store.GetServer(ctx, scopeTarget.ServerID); serverErr == nil {
				scopeTarget.ServerName = server.Name
			}
			err = s.SyncDHCPScope(ctx, scopeTarget.ServerID, scopeTarget.ScopeExternalID)
		}
	default:
		progress, err = s.SyncAll(ctx, taskID, taskType)
	}

	if err != nil {
		message := err.Error()
		payload := any(refreshTaskPayload(taskType, target, "刷新任务失败", message))
		if progress != nil {
			progress.Message = refreshProgressMessage(*progress)
			progress.CurrentAgent = ""
			progress.Error = message
			payload = progress
		}
		s.updateRefreshTask(ctx, taskID, "failed", payload)
		if progress != nil {
			_ = s.publishWithPayload(ctx, taskType, taskID, "failed", "刷新任务失败", payload)
		} else {
			_ = s.publish(ctx, taskType, taskID, "failed", "刷新任务失败")
		}
		s.logger.Warn("Refresh task failed", "task", taskID, "type", taskType, "error", err)
		return
	}
	payload := any(refreshTaskPayload(taskType, target, "刷新任务完成", ""))
	if progress != nil {
		progress.Message = refreshProgressMessage(*progress)
		progress.CurrentAgent = ""
		payload = progress
	}
	s.updateRefreshTask(ctx, taskID, "completed", payload)
	if progress != nil {
		_ = s.publishWithPayload(ctx, taskType, taskID, "success", "刷新任务完成", payload)
	} else {
		_ = s.publish(ctx, taskType, taskID, "success", "刷新任务完成")
	}
	_ = s.publish(ctx, "runtime.updated", taskID, "success", "运行态已更新")
}

func refreshTaskPayload(taskType string, target any, message, errorMessage string) map[string]any {
	payload := map[string]any{"message": message}
	switch taskType {
	case RefreshServerType:
		if serverTarget, ok := target.(*ServerTarget); ok && serverTarget != nil {
			payload["resourceType"] = "server"
			payload["resourceId"] = serverTarget.ServerID
			payload["resourceName"] = serverTarget.ServerName
			payload["serverId"] = serverTarget.ServerID
			if strings.TrimSpace(serverTarget.ServerName) != "" {
				payload["serverName"] = serverTarget.ServerName
			}
		}
	case RefreshDNSZoneType:
		if zoneTarget, ok := target.(*ZoneTarget); ok && zoneTarget != nil {
			payload["resourceType"] = "dns.zone"
			payload["resourceId"] = zoneTarget.ZoneID
			payload["resourceName"] = zoneTarget.ZoneName
			payload["serverId"] = zoneTarget.ServerID
			if strings.TrimSpace(zoneTarget.ServerName) != "" {
				payload["serverName"] = zoneTarget.ServerName
			}
		}
	case RefreshDHCPScopeType:
		if scopeTarget, ok := target.(*DHCPScopeTarget); ok && scopeTarget != nil {
			payload["resourceType"] = "dhcp.scope"
			payload["resourceId"] = scopeTarget.ScopeID
			payload["resourceName"] = scopeTarget.ScopeName
			payload["serverId"] = scopeTarget.ServerID
			if strings.TrimSpace(scopeTarget.ServerName) != "" {
				payload["serverName"] = scopeTarget.ServerName
			}
		}
	}
	if errorMessage != "" {
		payload["error"] = errorMessage
	}
	return payload
}

func (s *Service) SyncAll(ctx context.Context, taskID, taskType string) (*refreshProgressPayload, error) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]domain.Server, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server.AgentURL) != "" && shouldSyncServerForTask(taskType, server) {
			targets = append(targets, server)
		}
	}
	total := len(targets)
	concurrency := s.runtimeLimits(ctx).SyncConcurrency
	sem := make(chan struct{}, concurrency)
	var wg stdsync.WaitGroup
	var mu stdsync.Mutex
	messages := []string{}
	agentResults := []refreshAgentResult{}
	started := 0
	synced := 0
	failed := 0
	skipped := 0
	startedAt := localTaskTime(time.Now())
	publishProgress := func(currentAgent string, agentEvent *refreshAgentResult, publishEvent bool) {
		if taskID == "" || taskType == "" {
			return
		}
		payload := refreshProgressPayload{
			Message:       "刷新任务运行中",
			ResourceType:  "runtime",
			TotalAgents:   total,
			StartedAgents: started,
			SyncedAgents:  synced,
			FailedAgents:  failed,
			SkippedAgents: skipped,
			CurrentAgent:  currentAgent,
			StartedAt:     startedAt,
			AgentResults:  append([]refreshAgentResult(nil), agentResults...),
		}
		payload.Warn = refreshProgressWarn(payload.AgentResults)
		if agentEvent != nil {
			eventCopy := *agentEvent
			payload.AgentEvent = &eventCopy
		}
		s.updateRefreshTask(ctx, taskID, "running", payload)
		if publishEvent {
			_ = s.publishWithPayload(ctx, taskType, taskID, "progress", "刷新任务运行中", payload)
		}
	}
	publishProgress("", nil, false)
	for _, server := range targets {
		server := server
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			mu.Lock()
			started++
			publishProgress(server.Name, &refreshAgentResult{ID: server.ID, Name: server.Name, Status: "running"}, true)
			mu.Unlock()
			if err := s.syncServer(ctx, server); err != nil {
				mu.Lock()
				if isAgentSyncRunningError(err) {
					skipped++
					result := refreshAgentResult{ID: server.ID, Name: server.Name, Status: "skipped", Warn: server.Name + " 已处于正在同步中，跳过同步"}
					agentResults = append(agentResults, result)
					publishProgress("", &result, true)
					mu.Unlock()
					return
				}
				failed++
				messages = append(messages, server.Name+": "+err.Error())
				result := refreshAgentResult{ID: server.ID, Name: server.Name, Status: "failed", Error: err.Error()}
				agentResults = append(agentResults, result)
				publishProgress("", &result, true)
				mu.Unlock()
				return
			}
			mu.Lock()
			synced++
			result := refreshAgentResult{ID: server.ID, Name: server.Name, Status: "completed"}
			agentResults = append(agentResults, result)
			publishProgress("", &result, true)
			mu.Unlock()
		}()
	}
	wg.Wait()
	finishedAt := localTaskTime(time.Now())
	if len(messages) > 0 {
		message := strings.Join(messages, "; ")
		progress := refreshProgressPayload{
			ResourceType:  "runtime",
			TotalAgents:   total,
			StartedAgents: started,
			SyncedAgents:  synced,
			FailedAgents:  failed,
			SkippedAgents: skipped,
			StartedAt:     startedAt,
			FinishedAt:    finishedAt,
			AgentResults:  agentResults,
			Error:         message,
			Warn:          refreshProgressWarn(agentResults),
		}
		progress.Message = refreshProgressMessage(progress)
		return &progress, errors.New(message)
	}
	progress := refreshProgressPayload{
		ResourceType:  "runtime",
		TotalAgents:   total,
		StartedAgents: started,
		SyncedAgents:  synced,
		FailedAgents:  failed,
		SkippedAgents: skipped,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		AgentResults:  agentResults,
		Warn:          refreshProgressWarn(agentResults),
	}
	progress.Message = refreshProgressMessage(progress)
	return &progress, nil
}

func localTaskTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04:05")
}

func refreshProgressMessage(progress refreshProgressPayload) string {
	if progress.TotalAgents == 0 {
		return "暂无可同步的 Agent"
	}
	message := "刷新已完成 " + strconv.Itoa(progress.SyncedAgents) + "/" + strconv.Itoa(progress.TotalAgents) + "，异常 " + strconv.Itoa(progress.FailedAgents)
	if progress.SkippedAgents > 0 {
		message += "，跳过 " + strconv.Itoa(progress.SkippedAgents)
	}
	return message
}

func refreshProgressWarn(results []refreshAgentResult) string {
	warns := make([]string, 0)
	for _, result := range results {
		if strings.TrimSpace(result.Warn) != "" {
			warns = append(warns, result.Warn)
		}
	}
	return strings.Join(warns, "; ")
}

func shouldSyncServerForTask(taskType string, server domain.Server) bool {
	switch taskType {
	case RefreshDNSAllType:
		return isDNSServer(server)
	case RefreshDHCPAllType:
		return isDHCPServer(server)
	default:
		return true
	}
}

func (s *Service) SyncDNSZone(ctx context.Context, serverID, zoneName string) error {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	limits := s.runtimeLimits(ctx)
	agentCtx, cancel := context.WithTimeout(ctx, limits.AgentFullSyncTimeout)
	defer cancel()
	return s.syncDNSZoneRecords(agentCtx, server, domain.DNSZone{
		ID:       repository.DNSZoneID(server.ID, zoneName),
		Name:     zoneName,
		Type:     "Primary",
		Reverse:  isReverseDNSZoneName(zoneName),
		ServerID: server.ID,
	})
}

func (s *Service) SyncDHCPScope(ctx context.Context, serverID, scopeExternalID string) error {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	limits := s.runtimeLimits(ctx)
	agentCtx, cancel := context.WithTimeout(ctx, limits.AgentFullSyncTimeout)
	defer cancel()
	return s.syncDHCPScope(agentCtx, server, scopeExternalID)
}

func (s *Service) SyncServer(ctx context.Context, server domain.Server) error {
	return s.syncServer(ctx, server)
}

func (s *Service) syncServer(ctx context.Context, server domain.Server) error {
	limits := s.runtimeLimits(ctx)
	agentCtx, cancel := context.WithTimeout(ctx, limits.AgentFullSyncTimeout)
	defer cancel()
	return s.syncServerWithLimits(agentCtx, server, limits)
}

func (s *Service) syncServerWithLimits(ctx context.Context, server domain.Server, limits runtimeLimits) error {
	lockTTL := limits.AgentFullSyncTimeout + time.Minute
	lock, locked, err := s.realtime.TryLock(ctx, agentSyncLockKey(server.ID), lockTTL)
	if err != nil {
		s.logger.Warn("Acquire agent sync lock failed", "server", server.ID, "error", err)
	} else if !locked {
		return errAgentSyncRunning
	}
	if locked {
		defer func() {
			if err := s.realtime.Unlock(context.Background(), lock); err != nil {
				s.logger.Warn("Release agent sync lock failed", "server", server.ID, "error", err)
			}
		}()
	}
	s.markAgentSyncRunning(ctx, server, lockTTL)
	defer s.clearAgentSyncRunning(context.Background(), server.ID)

	status := "Online"
	started := time.Now()
	if err := s.agent.Health(ctx, server.AgentURL, server.APIKey, server.TLSInsecure); err != nil {
		status = "Offline"
		if healthUpdate, updateErr := s.store.UpdateServerHealth(ctx, server.ID, status, limits.AgentOfflineFailureCount); updateErr == nil && healthUpdate.BecameOffline() {
			s.cacheAgentHealth(ctx, server, status, err.Error(), healthUpdate.FailureCount, time.Since(started))
			s.notifyAgentOffline(ctx, server, err.Error())
		} else if updateErr == nil {
			s.cacheAgentHealth(ctx, server, status, err.Error(), healthUpdate.FailureCount, time.Since(started))
		}
		return err
	}
	if healthUpdate, updateErr := s.store.UpdateServerHealth(ctx, server.ID, status, limits.AgentOfflineFailureCount); updateErr == nil {
		s.cacheAgentHealth(ctx, server, status, "", healthUpdate.FailureCount, time.Since(started))
	}
	if err := s.store.DismissNotificationsBySource(ctx, notificationSourceAgentHealth, server.ID); err != nil {
		s.logger.Warn("Clear agent offline notification failed", "server", server.ID, "error", err)
	} else {
		s.invalidateUnreadNotificationCount(ctx)
	}

	var errs []string
	syncDNSRole := isDNSServer(server)
	syncDHCPRole := isDHCPServer(server)
	if !syncDNSRole && !syncDHCPRole {
		return fmt.Errorf("unsupported agent role: %s", strings.TrimSpace(server.Role))
	}
	if syncDNSRole {
		if err := s.syncDNS(ctx, server, limits); err != nil {
			errs = append(errs, "dns: "+err.Error())
		}
	}
	if syncDHCPRole {
		if err := s.syncDHCP(ctx, server, limits); err != nil {
			errs = append(errs, "dhcp: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func isAgentSyncRunningError(err error) bool {
	return errors.Is(err, errAgentSyncRunning)
}

func agentSyncRuntimeKey(serverID string) string {
	return "zonelease:runtime:agent-sync:" + strings.TrimSpace(serverID)
}

func agentSyncLockKey(serverID string) string {
	return "zonelease:lock:agent-sync:" + strings.TrimSpace(serverID)
}

func (s *Service) markAgentSyncRunning(ctx context.Context, server domain.Server, ttl time.Duration) {
	if strings.TrimSpace(server.ID) == "" {
		return
	}
	if ttl <= 0 {
		ttl = refreshTaskRuntimeTTL
	}
	if err := s.realtime.Set(ctx, agentSyncRuntimeKey(server.ID), map[string]any{
		"serverId":   server.ID,
		"serverName": server.Name,
		"startedAt":  time.Now().UTC().Format(time.RFC3339Nano),
	}, ttl); err != nil {
		s.logger.Warn("Cache agent sync runtime failed", "server", server.ID, "error", err)
	}
}

func (s *Service) clearAgentSyncRunning(ctx context.Context, serverID string) {
	if strings.TrimSpace(serverID) == "" {
		return
	}
	if err := s.realtime.Delete(ctx, agentSyncRuntimeKey(serverID)); err != nil {
		s.logger.Warn("Clear agent sync runtime failed", "server", serverID, "error", err)
	}
}

func (s *Service) IsAgentSyncRunning(ctx context.Context, serverID string) bool {
	return s.isAgentSyncRunning(ctx, serverID)
}

func (s *Service) isAgentSyncRunning(ctx context.Context, serverID string) bool {
	if strings.TrimSpace(serverID) == "" {
		return false
	}
	var payload map[string]any
	found, err := s.realtime.GetJSON(ctx, agentSyncRuntimeKey(serverID), &payload)
	if err != nil {
		s.logger.Warn("Read agent sync runtime failed", "server", serverID, "error", err)
		return false
	}
	return found
}

func (s *Service) syncDNS(ctx context.Context, server domain.Server, limits runtimeLimits) error {
	var zones []domain.DNSZone
	if err := s.agent.Get(ctx, server.AgentURL, server.APIKey, "/dns/zones", &zones, server.TLSInsecure); err != nil {
		return err
	}
	zones = filterSyncableDNSZones(zones)
	for i := range zones {
		zones[i].ServerID = server.ID
		zones[i].ID = repository.DNSZoneID(server.ID, zones[i].Name)
	}
	if err := s.store.ReplaceDNSZones(ctx, server.ID, zones); err != nil {
		return err
	}
	concurrency := limits.DNSRecordConcurrency
	sem := make(chan struct{}, concurrency)
	var wg stdsync.WaitGroup
	var mu stdsync.Mutex
	errs := []string{}
	for _, zone := range zones {
		zone.ServerID = server.ID
		zone.ID = repository.DNSZoneID(server.ID, zone.Name)
		wg.Add(1)
		go func(zone domain.DNSZone) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := s.syncDNSZoneRecords(ctx, server, zone); err != nil {
				mu.Lock()
				s.logger.Warn("DNS zone records sync failed", "server", server.ID, "zone", zone.Name, "error", err)
				errs = append(errs, zone.Name+": "+err.Error())
				mu.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *Service) syncDNSZoneRecords(ctx context.Context, server domain.Server, zone domain.DNSZone) error {
	records, err := s.fetchDNSZoneRecords(ctx, server, zone)
	if err != nil {
		return err
	}
	if !zone.Reverse {
		records = s.annotateDNSRecordsFromReverseZones(ctx, server, zone.Name, records)
	}
	return s.store.ReplaceDNSZoneRecords(ctx, zone, records)
}

func (s *Service) syncDHCP(ctx context.Context, server domain.Server, limits runtimeLimits) error {
	legacyAgent := s.isLegacyAgent(ctx, server)
	if legacyAgent {
		s.logger.Info("DHCP legacy agent detected, using granular sync", "server", server.ID)
		defer s.clearLegacyDHCPCache(server)
	}

	var scopes []domain.DHCPScope
	if err := s.agent.Get(ctx, server.AgentURL, server.APIKey, "/dhcp/scopes", &scopes, server.TLSInsecure); err != nil {
		return err
	}
	for i := range scopes {
		scopes[i].ServerID = server.ID
		scopes[i].ExternalID = strutil.FirstNonEmpty(scopes[i].ExternalID, scopes[i].ID, scopes[i].Subnet)
	}
	if err := s.store.ReplaceDHCPScopes(ctx, server.ID, scopes); err != nil {
		return err
	}

	concurrency := limits.DHCPScopeConcurrency
	workers := minPositive(concurrency, len(scopes))
	scopeJobs := make(chan domain.DHCPScope)
	var wg stdsync.WaitGroup
	var mu stdsync.Mutex
	errs := []string{}
	cancelled := false

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for scope := range scopeJobs {
				if ctx.Err() != nil {
					mu.Lock()
					cancelled = true
					mu.Unlock()
					return
				}
				scopeExclusions, scopeLeases, scopeReservations, err := s.fetchDHCPScopeDetails(ctx, server, scope.ExternalID, legacyAgent)
				if err != nil {
					mu.Lock()
					if ctx.Err() != nil {
						cancelled = true
					} else {
						errs = append(errs, scope.ExternalID+": "+err.Error())
					}
					mu.Unlock()
					continue
				}
				if err := s.store.ReplaceDHCPScopeSnapshot(ctx, server.ID, scope, scopeExclusions, scopeLeases, scopeReservations); err != nil {
					mu.Lock()
					if ctx.Err() != nil {
						cancelled = true
					} else {
						errs = append(errs, scope.ExternalID+": "+err.Error())
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				s.logger.Info("DHCP scope synced", "server", server.ID, "scope", scope.ExternalID)
				mu.Unlock()
			}
		}()
	}

sendScopes:
	for i := range scopes {
		if ctx.Err() != nil {
			mu.Lock()
			cancelled = true
			mu.Unlock()
			break
		}
		select {
		case scopeJobs <- scopes[i]:
		case <-ctx.Done():
			mu.Lock()
			cancelled = true
			mu.Unlock()
			break sendScopes
		}
	}
	close(scopeJobs)
	wg.Wait()
	if cancelled {
		return ctx.Err()
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (s *Service) clearLegacyDHCPCache(server domain.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var ignored map[string]any
	if err := s.agent.Post(ctx, server.AgentURL, server.APIKey, "/dhcp/cache/clear", nil, &ignored, server.TLSInsecure); err != nil && !agentNotFound(err) {
		s.logger.Warn("Clear legacy DHCP cache failed", "server", server.ID, "error", err)
	}
}

func agentNotFound(err error) bool {
	var statusErr agent.HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == 404
}

func (s *Service) isLegacyAgent(ctx context.Context, server domain.Server) bool {
	var health map[string]any
	if err := s.agent.Get(ctx, server.AgentURL, server.APIKey, "/health", &health, server.TLSInsecure); err != nil {
		return false
	}
	mode, _ := health["mode"].(string)
	return strings.EqualFold(strings.TrimSpace(mode), "legacy")
}

func (s *Service) syncDHCPScope(ctx context.Context, server domain.Server, scopeExternalID string) error {
	legacyAgent := s.isLegacyAgent(ctx, server)
	if legacyAgent {
		target, err := s.fetchLegacyDHCPScope(ctx, server, scopeExternalID)
		if err != nil {
			if agentNotFound(err) {
				return s.store.DeleteDHCPScopeByExternalID(ctx, server.ID, scopeExternalID)
			}
			return err
		}
		if strings.TrimSpace(target.ExternalID) == "" {
			return s.store.DeleteDHCPScopeByExternalID(ctx, server.ID, scopeExternalID)
		}
		exclusions, leases, reservations, err := s.fetchDHCPScopeDetails(ctx, server, target.ExternalID, legacyAgent)
		if err != nil {
			return err
		}
		return s.store.ReplaceDHCPScopeSnapshot(ctx, server.ID, target, exclusions, leases, reservations)
	}

	var scopes []domain.DHCPScope
	if err := s.agent.Get(ctx, server.AgentURL, server.APIKey, "/dhcp/scopes", &scopes, server.TLSInsecure); err != nil {
		return err
	}
	var target domain.DHCPScope
	for _, scope := range scopes {
		scope.ServerID = server.ID
		scope.ExternalID = strutil.FirstNonEmpty(scope.ExternalID, scope.ID, scope.Subnet)
		if scope.ExternalID == scopeExternalID {
			target = scope
			break
		}
	}
	if strings.TrimSpace(target.ExternalID) == "" {
		return s.store.DeleteDHCPScopeByExternalID(ctx, server.ID, scopeExternalID)
	}
	exclusions, leases, reservations, err := s.fetchDHCPScopeDetails(ctx, server, target.ExternalID, legacyAgent)
	if err != nil {
		return err
	}
	return s.store.ReplaceDHCPScopeSnapshot(ctx, server.ID, target, exclusions, leases, reservations)
}

func (s *Service) fetchLegacyDHCPScope(ctx context.Context, server domain.Server, scopeExternalID string) (domain.DHCPScope, error) {
	var scope domain.DHCPScope
	path := "/dhcp/scopes/" + url.PathEscape(scopeExternalID)
	if err := s.agent.Get(ctx, server.AgentURL, server.APIKey, path, &scope, server.TLSInsecure); err != nil {
		return domain.DHCPScope{}, err
	}
	scope.ServerID = server.ID
	scope.ExternalID = strutil.FirstNonEmpty(scope.ExternalID, scope.ID, scope.Subnet)
	return scope, nil
}

func (s *Service) fetchDHCPScopeDetails(ctx context.Context, server domain.Server, scopeExternalID string, legacyAgent bool) ([]domain.DHCPExclusion, []domain.DHCPLease, []domain.DHCPReservation, error) {
	if legacyAgent {
		return s.fetchLegacyDHCPScopeDetails(ctx, server, scopeExternalID)
	}
	var exclusions []domain.DHCPExclusion
	exclusionPath := "/dhcp/scopes/" + url.PathEscape(scopeExternalID) + "/exclusions"
	var leases []domain.DHCPLease
	path := "/dhcp/scopes/" + url.PathEscape(scopeExternalID) + "/leases"
	var reservations []domain.DHCPReservation
	reservationPath := "/dhcp/scopes/" + url.PathEscape(scopeExternalID) + "/reservations"

	var wg stdsync.WaitGroup
	var exclusionErr error
	var leaseErr error
	var reservationErr error
	wg.Add(3)
	go func() {
		defer wg.Done()
		exclusionErr = s.agent.Get(ctx, server.AgentURL, server.APIKey, exclusionPath, &exclusions, server.TLSInsecure)
	}()
	go func() {
		defer wg.Done()
		leaseErr = s.agent.Get(ctx, server.AgentURL, server.APIKey, path, &leases, server.TLSInsecure)
	}()
	go func() {
		defer wg.Done()
		reservationErr = s.agent.Get(ctx, server.AgentURL, server.APIKey, reservationPath, &reservations, server.TLSInsecure)
	}()
	wg.Wait()
	if exclusionErr != nil && !agentNotFound(exclusionErr) {
		return nil, nil, nil, exclusionErr
	}
	if leaseErr != nil {
		return nil, nil, nil, leaseErr
	}
	if reservationErr != nil {
		return nil, nil, nil, reservationErr
	}

	for i := range exclusions {
		exclusions[i].ScopeID = scopeExternalID
		exclusions[i].ExternalID = strutil.FirstNonEmpty(exclusions[i].ExternalID, exclusions[i].ID, exclusions[i].StartIP+"-"+exclusions[i].EndIP)
	}
	for i := range leases {
		leases[i].ScopeID = scopeExternalID
		leases[i].ExternalID = strutil.FirstNonEmpty(leases[i].ExternalID, leases[i].ID, leases[i].IP)
	}
	for i := range reservations {
		reservations[i].ScopeID = scopeExternalID
		reservations[i].ExternalID = strutil.FirstNonEmpty(reservations[i].ExternalID, reservations[i].ID, reservations[i].IP)
	}
	return exclusions, leases, reservations, nil
}

func (s *Service) fetchLegacyDHCPScopeDetails(ctx context.Context, server domain.Server, scopeExternalID string) ([]domain.DHCPExclusion, []domain.DHCPLease, []domain.DHCPReservation, error) {
	var detail struct {
		Exclusions   []domain.DHCPExclusion   `json:"exclusions"`
		Leases       []domain.DHCPLease       `json:"leases"`
		Reservations []domain.DHCPReservation `json:"reservations"`
	}
	path := "/dhcp/scopes/" + url.PathEscape(scopeExternalID) + "/details"
	if err := s.agent.Get(ctx, server.AgentURL, server.APIKey, path, &detail, server.TLSInsecure); err != nil {
		return nil, nil, nil, err
	}
	for i := range detail.Exclusions {
		detail.Exclusions[i].ScopeID = scopeExternalID
		detail.Exclusions[i].ExternalID = strutil.FirstNonEmpty(detail.Exclusions[i].ExternalID, detail.Exclusions[i].ID, detail.Exclusions[i].StartIP+"-"+detail.Exclusions[i].EndIP)
	}
	for i := range detail.Leases {
		detail.Leases[i].ScopeID = scopeExternalID
		detail.Leases[i].ExternalID = strutil.FirstNonEmpty(detail.Leases[i].ExternalID, detail.Leases[i].ID, detail.Leases[i].IP)
	}
	for i := range detail.Reservations {
		detail.Reservations[i].ScopeID = scopeExternalID
		detail.Reservations[i].ExternalID = strutil.FirstNonEmpty(detail.Reservations[i].ExternalID, detail.Reservations[i].ID, detail.Reservations[i].IP)
	}
	return detail.Exclusions, detail.Leases, detail.Reservations, nil
}

func (s *Service) publish(ctx context.Context, eventType, taskID, status, message string) error {
	return s.realtime.PublishRefresh(ctx, realtime.RefreshEvent{Type: eventType, TaskID: taskID, Status: status, Message: message})
}

func (s *Service) publishWithPayload(ctx context.Context, eventType, taskID, status, message string, payload any) error {
	return s.realtime.PublishRefresh(ctx, realtime.RefreshEvent{Type: eventType, TaskID: taskID, Status: status, Message: message, Payload: payload})
}

func (s *Service) updateRefreshTask(ctx context.Context, taskID, status string, payload any) {
	if err := s.store.UpdateRefreshTask(ctx, taskID, status, payload); err != nil {
		s.logger.Warn("Update refresh task failed", "task", taskID, "status", status, "error", err)
	}
	if err := s.realtime.Set(ctx, refreshTaskRuntimeKey(taskID), map[string]any{
		"status":  status,
		"payload": payload,
	}, refreshTaskRuntimeTTL); err != nil {
		s.logger.Warn("Cache refresh task runtime failed", "task", taskID, "status", status, "error", err)
	}
}

func refreshTaskRuntimeKey(taskID string) string {
	return "zonelease:runtime:refresh-task:" + strings.TrimSpace(taskID)
}

func agentHealthRuntimeKey(serverID string) string {
	return "zonelease:runtime:agent-health:" + strings.TrimSpace(serverID)
}

func (s *Service) invalidateUnreadNotificationCount(ctx context.Context) {
	if err := s.realtime.Delete(ctx, unreadNotificationCountCacheKey); err != nil {
		s.logger.Warn("Invalidate unread notification count cache failed", "error", err)
	}
}

func isDNSServer(server domain.Server) bool {
	role := strings.ToLower(strings.TrimSpace(server.Role))
	return role == "dns"
}

func isDHCPServer(server domain.Server) bool {
	role := strings.ToLower(strings.TrimSpace(server.Role))
	return role == "dhcp"
}

func isReverseDNSZoneName(name string) bool {
	value := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(value, ".in-addr.arpa") || strings.HasSuffix(value, ".ip6.arpa")
}

func (s *Service) runtimeLimits(ctx context.Context) runtimeLimits {
	defaults := domain.DefaultSystemBaseConfig()
	limits := runtimeLimits{
		SyncConcurrency:          defaults.RuntimeSyncConcurrency,
		DNSRecordConcurrency:     defaults.DNSRecordConcurrency,
		DHCPScopeConcurrency:     defaults.DHCPScopeConcurrency,
		AgentOfflineFailureCount: defaults.AgentOfflineFailureCount,
		AgentHealthConcurrency:   defaults.AgentHealthCheckConcurrency,
		AgentOperationTimeout:    time.Duration(defaults.AgentOperationTimeoutSeconds) * time.Second,
		AgentFullSyncTimeout:     time.Duration(defaults.AgentFullSyncTimeoutSeconds) * time.Second,
	}
	base, err := s.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return limits
	}
	base = domain.NormalizeSystemBaseConfig(base)
	limits.SyncConcurrency = base.RuntimeSyncConcurrency
	limits.DNSRecordConcurrency = base.DNSRecordConcurrency
	limits.DHCPScopeConcurrency = base.DHCPScopeConcurrency
	limits.AgentOfflineFailureCount = base.AgentOfflineFailureCount
	limits.AgentHealthConcurrency = base.AgentHealthCheckConcurrency
	limits.AgentOperationTimeout = time.Duration(base.AgentOperationTimeoutSeconds) * time.Second
	limits.AgentFullSyncTimeout = time.Duration(base.AgentFullSyncTimeoutSeconds) * time.Second
	return limits
}

func (s *Service) healthCheckInterval(ctx context.Context) time.Duration {
	defaults := domain.DefaultSystemBaseConfig()
	interval := time.Duration(defaults.AgentHealthCheckIntervalMinutes) * time.Minute
	base, err := s.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return interval
	}
	base = domain.NormalizeSystemBaseConfig(base)
	return time.Duration(base.AgentHealthCheckIntervalMinutes) * time.Minute
}

func (s *Service) refreshTaskLockTTL(ctx context.Context) time.Duration {
	ttl := s.runtimeLimits(ctx).AgentFullSyncTimeout + time.Minute
	if ttl <= time.Minute {
		return 2 * time.Minute
	}
	return ttl
}

func RefreshTaskLockKey(taskType string, target any) string {
	return refreshTaskLockKey(taskType, target)
}

func refreshTaskLockKey(taskType string, target any) string {
	switch taskType {
	case RefreshAllType:
		return "zonelease:lock:refresh:all"
	case RefreshDNSAllType:
		return "zonelease:lock:refresh:dns-all"
	case RefreshDHCPAllType:
		return "zonelease:lock:refresh:dhcp-all"
	case RefreshServerType:
		serverTarget, ok := target.(*ServerTarget)
		if !ok || strings.TrimSpace(serverTarget.ServerID) == "" {
			return ""
		}
		return "zonelease:lock:refresh:server:" + strings.TrimSpace(serverTarget.ServerID)
	case RefreshDNSZoneType:
		zoneTarget, ok := target.(*ZoneTarget)
		if !ok || strings.TrimSpace(zoneTarget.ServerID) == "" || strings.TrimSpace(zoneTarget.ZoneName) == "" {
			return ""
		}
		return "zonelease:lock:refresh:dns-zone:" + strings.TrimSpace(zoneTarget.ServerID) + ":" + strings.ToLower(strings.TrimSpace(zoneTarget.ZoneName))
	case RefreshDHCPScopeType:
		scopeTarget, ok := target.(*DHCPScopeTarget)
		if !ok || strings.TrimSpace(scopeTarget.ServerID) == "" || strings.TrimSpace(scopeTarget.ScopeExternalID) == "" {
			return ""
		}
		return "zonelease:lock:refresh:dhcp-scope:" + strings.TrimSpace(scopeTarget.ServerID) + ":" + strings.TrimSpace(scopeTarget.ScopeExternalID)
	default:
		return ""
	}
}

func minPositive(value, limit int) int {
	if value <= 0 {
		value = 1
	}
	if limit > 0 && value > limit {
		return limit
	}
	return value
}

func maxPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
