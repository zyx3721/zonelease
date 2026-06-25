package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"zonelease/dns-agent/internal/config"
	"zonelease/dns-agent/internal/dns"
)

type Server struct {
	cfg      config.Config
	provider dns.Provider
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

func New(cfg config.Config, provider dns.Provider, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, provider: provider, logger: logger}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /dns/zones", s.listZones)
	mux.HandleFunc("POST /dns/zones", s.createZone)
	mux.HandleFunc("POST /dns/records/query", s.queryRecords)
	mux.HandleFunc("POST /dns/records/create", s.createRecord)
	mux.HandleFunc("POST /dns/records/delete", s.deleteRecord)
	mux.HandleFunc("POST /dns/records/update", s.updateRecord)
	mux.HandleFunc("GET /dns/zones/", s.zoneRoute)
	mux.HandleFunc("POST /dns/zones/", s.zoneRoute)
	mux.HandleFunc("PUT /dns/zones/", s.zoneRoute)
	mux.HandleFunc("DELETE /dns/zones/", s.zoneRoute)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := time.Now().UTC().Format("20060102150405.000000000")
		w.Header().Set("X-Request-ID", requestID)
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("DNS agent request panicked", "method", r.Method, "path", r.URL.Path, "requestId", requestID, "duration", time.Since(started).String(), "panic", recovered)
				writeError(w, http.StatusInternalServerError, requestID, "AGENT_PANIC", "agent request failed unexpectedly")
				return
			}
			s.logger.Info("DNS agent request completed", "method", r.Method, "path", r.URL.Path, "requestId", requestID, "duration", time.Since(started).String())
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
		"role":                     "dns-agent",
		"powerShellTimeoutSeconds": s.cfg.PowerShellTimeoutSeconds,
	}})
}

func (s *Server) listZones(w http.ResponseWriter, r *http.Request) {
	items, err := s.provider.ListZones(r.Context())
	respond(w, r, items, err)
}

func (s *Server) createZone(w http.ResponseWriter, r *http.Request) {
	var body dns.Zone
	if !decode(w, r, &body) {
		return
	}
	respond(w, r, map[string]bool{"created": true}, s.provider.CreateZone(r.Context(), body))
}

func (s *Server) queryRecords(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Zone string `json:"zone"`
	}
	if !decode(w, r, &body) {
		return
	}
	zone := strings.TrimSpace(body.Zone)
	if zone == "" {
		writeError(w, http.StatusBadRequest, requestID(r), "BAD_REQUEST", "zone is required")
		return
	}
	items, err := s.provider.ListRecords(r.Context(), zone)
	respond(w, r, items, err)
}

func (s *Server) createRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Zone   string     `json:"zone"`
		Record dns.Record `json:"record"`
	}
	if !decode(w, r, &body) {
		return
	}
	zone := strings.TrimSpace(body.Zone)
	if zone == "" {
		writeError(w, http.StatusBadRequest, requestID(r), "BAD_REQUEST", "zone is required")
		return
	}
	result, err := s.provider.CreateRecord(r.Context(), zone, body.Record)
	respond(w, r, result, err)
}

func (s *Server) deleteRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Zone   string     `json:"zone"`
		Record dns.Record `json:"record"`
	}
	if !decode(w, r, &body) {
		return
	}
	zone := strings.TrimSpace(body.Zone)
	if zone == "" {
		writeError(w, http.StatusBadRequest, requestID(r), "BAD_REQUEST", "zone is required")
		return
	}
	respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteRecord(r.Context(), zone, body.Record))
}

func (s *Server) updateRecord(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Zone   string           `json:"zone"`
		Update dns.RecordUpdate `json:"update"`
	}
	if !decode(w, r, &body) {
		return
	}
	zone := strings.TrimSpace(body.Zone)
	if zone == "" {
		writeError(w, http.StatusBadRequest, requestID(r), "BAD_REQUEST", "zone is required")
		return
	}
	result, err := s.provider.UpdateRecord(r.Context(), zone, body.Update)
	respond(w, r, result, err)
}

func (s *Server) deleteZone(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/dns/zones/"), "/")
	respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteZone(r.Context(), name))
}

func (s *Server) zoneRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/dns/zones/"), "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodDelete {
		respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteZone(r.Context(), parts[0]))
		return
	}
	if len(parts) < 2 || parts[1] != "records" {
		writeError(w, http.StatusNotFound, requestID(r), "NOT_FOUND", "route not found")
		return
	}
	zone := parts[0]
	if len(parts) == 2 && r.Method == http.MethodGet {
		items, err := s.provider.ListRecords(r.Context(), zone)
		respond(w, r, items, err)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		var body dns.Record
		if !decode(w, r, &body) {
			return
		}
		result, err := s.provider.CreateRecord(r.Context(), zone, body)
		respond(w, r, result, err)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPut {
		var body dns.RecordUpdate
		if !decode(w, r, &body) {
			return
		}
		result, err := s.provider.UpdateRecord(r.Context(), zone, body)
		respond(w, r, result, err)
		return
	}
	if len(parts) == 4 && r.Method == http.MethodDelete {
		record := dns.Record{Name: parts[3], Type: parts[2], Value: r.URL.Query().Get("value"), CreatePTR: r.URL.Query().Get("createPtr") == "true"}
		respond(w, r, map[string]bool{"deleted": true}, s.provider.DeleteRecord(r.Context(), zone, record))
		return
	}
	writeError(w, http.StatusNotFound, requestID(r), "NOT_FOUND", "route not found")
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
		slog.Warn("DNS agent request failed", "method", r.Method, "path", r.URL.Path, "requestId", requestID(r), "error", err)
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
