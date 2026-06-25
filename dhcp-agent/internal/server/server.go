package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"zonelease/dhcp-agent/internal/config"
	"zonelease/dhcp-agent/internal/dhcp"
)

type Server struct {
	cfg      config.Config
	provider dhcp.Provider
	logger   *slog.Logger
}

type envelope struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *apiError `json:"error,omitempty"`
	Request string    `json:"requestId"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(cfg config.Config, provider dhcp.Provider, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, provider: provider, logger: logger}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /dhcp/probe", s.probe)
	mux.HandleFunc("GET /dhcp/scopes", s.listScopes)
	mux.HandleFunc("POST /dhcp/scopes", s.createScope)
	mux.HandleFunc("POST /dhcp/scopes/state", s.setScopeState)
	mux.HandleFunc("POST /dhcp/scopes/", s.scopeRoute)
	mux.HandleFunc("PUT /dhcp/scopes/", s.scopeRoute)
	mux.HandleFunc("DELETE /dhcp/scopes/", s.scopeRoute)
	mux.HandleFunc("GET /dhcp/scopes/", s.scopeRoute)
	mux.HandleFunc("POST /dhcp/exclusions", s.createExclusion)
	mux.HandleFunc("POST /dhcp/exclusions/delete", s.deleteExclusion)
	mux.HandleFunc("POST /dhcp/leases/release", s.releaseLease)
	mux.HandleFunc("POST /dhcp/reservations", s.createReservation)
	mux.HandleFunc("POST /dhcp/reservations/delete", s.deleteReservationByBody)
	mux.HandleFunc("POST /dhcp/reservations/update", s.updateReservationByBody)
	mux.HandleFunc("PUT /dhcp/reservations/", s.updateReservation)
	mux.HandleFunc("DELETE /dhcp/reservations/", s.deleteReservation)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := time.Now().UTC().Format("20060102150405.000000000")
		w.Header().Set("X-Request-ID", requestID)
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("DHCP agent request panicked", "method", r.Method, "path", r.URL.Path, "requestId", requestID, "duration", time.Since(started).String(), "panic", recovered)
				writeError(w, http.StatusInternalServerError, requestID, "AGENT_PANIC", "agent request failed unexpectedly")
				return
			}
			s.logger.Info("DHCP agent request completed", "method", r.Method, "path", r.URL.Path, "requestId", requestID, "duration", time.Since(started).String())
		}()
		if r.URL.Path != "/health" && !s.cfg.AllowAnonymous && strings.TrimSpace(s.cfg.APIKey) != "" && r.Header.Get("X-API-Key") != s.cfg.APIKey {
			writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "invalid api key")
			return
		}
		next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), requestID)))
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Success: true, Request: requestID(r), Data: map[string]any{
		"status":                   "ok",
		"role":                     "dhcp-agent",
		"powerShellTimeoutSeconds": s.cfg.PowerShellTimeoutSeconds,
	}})
}

func (s *Server) listScopes(w http.ResponseWriter, r *http.Request) {
	items, err := s.provider.ListScopes(r.Context())
	respond(w, r, items, err)
}

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	respond(w, r, map[string]string{"status": "ok"}, s.provider.Probe(r.Context()))
}

func (s *Server) createScope(w http.ResponseWriter, r *http.Request) {
	var body dhcp.Scope
	if !decode(w, r, &body) {
		return
	}
	result, err := s.provider.CreateScope(r.Context(), body)
	respond(w, r, result, err)
}

func (s *Server) setScopeState(w http.ResponseWriter, r *http.Request) {
	var body dhcp.ScopeState
	if !decode(w, r, &body) {
		return
	}
	respond(w, r, map[string]bool{"active": body.Active}, s.provider.SetScopeState(r.Context(), body.ScopeID, body.Active))
}

func (s *Server) scopeRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/dhcp/scopes/"), "/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, requestID(r), "NOT_FOUND", "route not found")
		return
	}
	scopeID := parts[0]
	if len(parts) == 1 && r.Method == http.MethodDelete {
		respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteScope(r.Context(), scopeID))
		return
	}
	if len(parts) == 1 && r.Method == http.MethodPut {
		var body dhcp.Scope
		if !decode(w, r, &body) {
			return
		}
		body.ID = scopeID
		result, err := s.provider.UpdateScope(r.Context(), body)
		respond(w, r, result, err)
		return
	}
	if len(parts) == 2 && parts[1] == "activate" && r.Method == http.MethodPost {
		respond(w, r, map[string]bool{"active": true}, s.provider.SetScopeState(r.Context(), scopeID, true))
		return
	}
	if len(parts) == 2 && parts[1] == "deactivate" && r.Method == http.MethodPost {
		respond(w, r, map[string]bool{"active": false}, s.provider.SetScopeState(r.Context(), scopeID, false))
		return
	}
	if len(parts) == 2 && parts[1] == "leases" && r.Method == http.MethodGet {
		items, err := s.provider.ListLeases(r.Context(), scopeID)
		respond(w, r, items, err)
		return
	}
	if len(parts) == 2 && parts[1] == "reservations" && r.Method == http.MethodGet {
		items, err := s.provider.ListReservations(r.Context(), scopeID)
		respond(w, r, items, err)
		return
	}
	if len(parts) == 2 && parts[1] == "exclusions" && r.Method == http.MethodGet {
		items, err := s.provider.ListExclusions(r.Context(), scopeID)
		respond(w, r, items, err)
		return
	}
	if len(parts) == 3 && parts[1] == "leases" && r.Method == http.MethodDelete {
		respond(w, r, map[string]bool{"released": true}, s.provider.ReleaseLease(r.Context(), scopeID, parts[2]))
		return
	}
	writeError(w, http.StatusNotFound, requestID(r), "NOT_FOUND", "route not found")
}

func (s *Server) releaseLease(w http.ResponseWriter, r *http.Request) {
	var body dhcp.LeaseRef
	if !decode(w, r, &body) {
		return
	}
	respond(w, r, map[string]bool{"released": true}, s.provider.ReleaseLease(r.Context(), body.ScopeID, body.IP))
}

func (s *Server) createExclusion(w http.ResponseWriter, r *http.Request) {
	var body dhcp.Exclusion
	if !decode(w, r, &body) {
		return
	}
	result, err := s.provider.CreateExclusion(r.Context(), body)
	respond(w, r, result, err)
}

func (s *Server) deleteExclusion(w http.ResponseWriter, r *http.Request) {
	var body dhcp.ExclusionRef
	if !decode(w, r, &body) {
		return
	}
	respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteExclusion(r.Context(), body.ScopeID, body.StartIP, body.EndIP))
}

func (s *Server) createReservation(w http.ResponseWriter, r *http.Request) {
	var body dhcp.Reservation
	if !decode(w, r, &body) {
		return
	}
	result, err := s.provider.CreateReservation(r.Context(), body)
	respond(w, r, result, err)
}

func (s *Server) updateReservationByBody(w http.ResponseWriter, r *http.Request) {
	var body dhcp.ReservationUpdate
	if !decode(w, r, &body) {
		return
	}
	result, err := s.provider.UpdateReservation(r.Context(), body)
	respond(w, r, result, err)
}

func (s *Server) updateReservation(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/dhcp/reservations/"), "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, requestID(r), "NOT_FOUND", "route not found")
		return
	}
	var body dhcp.Reservation
	if !decode(w, r, &body) {
		return
	}
	update := dhcp.ReservationUpdate{
		Old: dhcp.Reservation{ScopeID: parts[0], IP: parts[1], MAC: body.MAC, Name: body.Name},
		New: body,
	}
	if update.New.ScopeID == "" {
		update.New.ScopeID = parts[0]
	}
	if update.New.IP == "" {
		update.New.IP = parts[1]
	}
	result, err := s.provider.UpdateReservation(r.Context(), update)
	respond(w, r, result, err)
}

func (s *Server) deleteReservation(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/dhcp/reservations/"), "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, requestID(r), "NOT_FOUND", "route not found")
		return
	}
	respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteReservation(r.Context(), parts[0], parts[1]))
}

func (s *Server) deleteReservationByBody(w http.ResponseWriter, r *http.Request) {
	var body dhcp.LeaseRef
	if !decode(w, r, &body) {
		return
	}
	respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteReservation(r.Context(), body.ScopeID, body.IP))
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, requestID(r), "BAD_REQUEST", err.Error())
		return false
	}
	return true
}

func respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err != nil {
		slog.Warn("DHCP agent request failed", "method", r.Method, "path", r.URL.Path, "requestId", requestID(r), "error", err)
		writeError(w, http.StatusInternalServerError, requestID(r), "AGENT_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envelope{Success: true, Request: requestID(r), Data: data})
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, requestID, code, message string) {
	writeJSON(w, status, envelope{Success: false, Request: requestID, Error: &apiError{Code: code, Message: message}})
}
