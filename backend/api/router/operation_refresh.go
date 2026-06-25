package router

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type operationRefreshKind string

const (
	operationRefreshDNSZone   operationRefreshKind = "dns.zone"
	operationRefreshDHCPScope operationRefreshKind = "dhcp.scope"
)

type operationRefreshTarget struct {
	Kind            operationRefreshKind
	ServerID        string
	ZoneID          string
	ZoneName        string
	ScopeExternalID string
	ScopeName       string
}

type operationRefreshScheduler struct {
	router  *Router
	logger  *slog.Logger
	mu      sync.Mutex
	targets map[string]*operationRefreshState
}

type operationRefreshState struct {
	target operationRefreshTarget
	active int
	dirty  bool
	timer  *time.Timer
}

func newOperationRefreshScheduler(router *Router) *operationRefreshScheduler {
	return &operationRefreshScheduler{
		router:  router,
		logger:  router.logger,
		targets: map[string]*operationRefreshState{},
	}
}

func (s *operationRefreshScheduler) begin(target operationRefreshTarget) func() {
	key := target.key()
	if key == "" {
		return func() {}
	}
	s.mu.Lock()
	state := s.stateForLocked(key, target)
	state.active++
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	s.mu.Unlock()
	return func() {
		s.finish(target)
	}
}

func (s *operationRefreshScheduler) finish(target operationRefreshTarget) {
	key := target.key()
	if key == "" {
		return
	}
	s.mu.Lock()
	state := s.stateForLocked(key, target)
	if state.active > 0 {
		state.active--
	}
	if state.active == 0 && state.dirty {
		s.armLocked(key, state)
	}
	s.mu.Unlock()
}

func (s *operationRefreshScheduler) markDirty(target operationRefreshTarget) {
	key := target.key()
	if key == "" {
		return
	}
	s.mu.Lock()
	state := s.stateForLocked(key, target)
	state.dirty = true
	if state.active == 0 {
		s.armLocked(key, state)
	}
	s.mu.Unlock()
}

func (s *operationRefreshScheduler) stateForLocked(key string, target operationRefreshTarget) *operationRefreshState {
	state := s.targets[key]
	if state == nil {
		state = &operationRefreshState{}
		s.targets[key] = state
	}
	state.target = target
	return state
}

func (s *operationRefreshScheduler) armLocked(key string, state *operationRefreshState) {
	if state.timer != nil {
		state.timer.Stop()
	}
	delay := s.router.operationRefreshDelay(context.Background())
	target := state.target
	state.timer = time.AfterFunc(delay, func() {
		s.fire(key, target)
	})
}

func (s *operationRefreshScheduler) fire(key string, target operationRefreshTarget) {
	s.mu.Lock()
	state := s.targets[key]
	if state == nil {
		s.mu.Unlock()
		return
	}
	if state.active > 0 {
		s.armLocked(key, state)
		s.mu.Unlock()
		return
	}
	delete(s.targets, key)
	s.mu.Unlock()

	lockKey := "zonelease:lock:operation-refresh:" + key
	lock, locked, err := s.router.realtime.TryLock(context.Background(), lockKey, s.router.operationRefreshDelay(context.Background())+time.Minute)
	if err != nil {
		s.logger.Warn("Acquire operation refresh lock failed", "target", key, "error", err)
	} else if !locked {
		s.logger.Info("Skip operation refresh because another instance holds the lock", "target", key)
		return
	} else {
		defer func() {
			if err := s.router.realtime.Unlock(context.Background(), lock); err != nil {
				s.logger.Warn("Release operation refresh lock failed", "target", key, "error", err)
			}
		}()
	}

	var refreshErr error
	switch target.Kind {
	case operationRefreshDNSZone:
		_, refreshErr = s.router.enqueueZoneRefresh(target.ServerID, target.ZoneID, target.ZoneName, "")
	case operationRefreshDHCPScope:
		_, refreshErr = s.router.enqueueDHCPScopeRefresh(target.ServerID, target.ScopeExternalID, target.ScopeName, "")
	}
	if refreshErr != nil {
		s.logger.Warn("Operation refresh enqueue failed", "target", key, "error", refreshErr)
	}
}

func (target operationRefreshTarget) key() string {
	switch target.Kind {
	case operationRefreshDNSZone:
		if target.ServerID == "" || target.ZoneName == "" {
			return ""
		}
		return string(target.Kind) + ":" + target.ServerID + ":" + target.ZoneName
	case operationRefreshDHCPScope:
		if target.ServerID == "" || target.ScopeExternalID == "" {
			return ""
		}
		return string(target.Kind) + ":" + target.ServerID + ":" + target.ScopeExternalID
	default:
		return ""
	}
}
