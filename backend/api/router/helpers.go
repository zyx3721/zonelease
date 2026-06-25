package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
	"zonelease/backend/internal/service/auth"
)

type contextKey string

const userContextKey contextKey = "zonelease-user"

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (r *Router) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		origin := r.cfg.CORS.Origin
		if origin == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if req.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func (r *Router) withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, req)
		r.logger.Info("Request handled", "method", req.Method, "path", req.URL.Path, "elapsedMs", time.Since(started).Milliseconds())
	})
}

func (r *Router) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		token := bearerToken(req)
		session, err := r.auth.Validate(req.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
			return
		}
		ctx := context.WithValue(req.Context(), userContextKey, session.User)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

func currentUser(req *http.Request) domain.User {
	user, _ := req.Context().Value(userContextKey).(domain.User)
	return user
}

func bearerToken(req *http.Request) string {
	value := req.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func decode(w http.ResponseWriter, req *http.Request, dst any) bool {
	defer req.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是合法 JSON")
		return false
	}
	return true
}

func pathID(path, prefix string) string {
	id := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

func pathIDAction(path, prefix string) (string, string) {
	value := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}

func (r *Router) writeAudit(req *http.Request, action, target, module, result string, metadata map[string]any) {
	user := currentUser(req)
	_ = r.store.WriteAudit(req.Context(), user.ID, user.Username, action, target, module, result, auditMetadata(metadata), repository.ClientIP(req))
}

func auditMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	detail := ""
	if data, err := json.Marshal(metadata); err == nil {
		detail = string(data)
	}
	return detail
}

func statusFromErr(err error) int {
	if errors.Is(err, repository.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrInvalidSession) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, auth.ErrInvalidResetToken) || errors.Is(err, auth.ErrInvalidResetCaptcha) || errors.Is(err, auth.ErrResetUnavailable) || errors.Is(err, auth.ErrResetCodeMismatch) || errors.Is(err, auth.ErrOldPasswordMismatch) {
		return http.StatusBadRequest
	}
	if errors.Is(err, auth.ErrResetChannelMissing) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, auth.ErrResetCodeCooldown) || errors.Is(err, auth.ErrResetCodeRateLimited) {
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}
