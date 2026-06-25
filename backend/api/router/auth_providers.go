package router

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
	authsvc "zonelease/backend/internal/service/auth"
)

var authProviderIDs = map[string]struct{}{"ldap": {}}

type authProviderRequest struct {
	Name    string         `json:"name"`
	Enabled bool           `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (r *Router) publicAuthProviders(w http.ResponseWriter, req *http.Request) {
	items, err := r.store.ListEnabledPublicAuthProviders(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_auth_providers_failed", "读取认证方式失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (r *Router) listAuthProviders(w http.ResponseWriter, req *http.Request) {
	items, err := r.store.ListAuthProviders(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_auth_providers_failed", "读取认证配置失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": redactAuthProviders(items), "total": len(items)})
}

func (r *Router) authProviderRoute(w http.ResponseWriter, req *http.Request) {
	id, action, ok := parseSettingsResourcePath(req.URL.Path, "/api/settings/auth-providers/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if req.Method == http.MethodPut && action == "" {
		r.updateAuthProvider(w, req, id)
		return
	}
	if req.Method == http.MethodPost && action == "test" {
		r.testAuthProvider(w, req, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不支持")
}

func (r *Router) updateAuthProvider(w http.ResponseWriter, req *http.Request, id string) {
	if !isAuthProviderID(id) {
		writeError(w, http.StatusNotFound, "auth_provider_not_found", "认证配置不存在")
		return
	}
	var body authProviderRequest
	if !decode(w, req, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_auth_provider_name", "显示名称不能为空")
		return
	}
	previous, _ := r.store.GetAuthProvider(req.Context(), id)
	config, err := sanitizeAuthProviderConfigWithPrevious(id, body.Config, configMap(previous.Config), body.Enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_auth_provider_config", err.Error())
		return
	}
	item, err := r.store.UpsertAuthProvider(req.Context(), id, "ldap", name, body.Enabled, config)
	if err != nil {
		r.logger.Error("Save auth provider failed", "error", err, "provider", id)
		writeError(w, http.StatusInternalServerError, "save_auth_provider_failed", "保存认证配置失败")
		return
	}
	r.writeAudit(req, "settings.auth_provider.update", id, "System", "success", map[string]any{"provider": id, "name": item.Name, "enabled": item.Enabled})
	writeJSON(w, http.StatusOK, redactAuthProvider(item))
}

func (r *Router) testAuthProvider(w http.ResponseWriter, req *http.Request, id string) {
	if !isAuthProviderID(id) {
		writeError(w, http.StatusNotFound, "auth_provider_not_found", "认证配置不存在")
		return
	}
	provider, err := r.store.GetAuthProvider(req.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "auth_provider_not_found", "认证配置不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "get_auth_provider_failed", "读取认证配置失败")
		return
	}
	result, err := authsvc.TestLDAPProvider(req.Context(), provider)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "auth_provider_test_failed", authsvc.LDAPUserMessage(err))
		return
	}
	r.writeAudit(req, "settings.auth_provider.test", id, "System", "success", map[string]any{"provider": id, "matchedUsers": result.MatchedUsers})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "matchedUsers": result.MatchedUsers})
}

func isAuthProviderID(id string) bool {
	_, ok := authProviderIDs[id]
	return ok
}

func sanitizeAuthProviderConfigWithPrevious(id string, config map[string]any, previous map[string]any, enabled bool) (map[string]any, error) {
	if id != "ldap" {
		return nil, fmt.Errorf("不支持的认证配置")
	}
	if config == nil {
		config = map[string]any{}
	}
	for key, value := range config {
		if text, ok := value.(string); ok {
			config[key] = strings.TrimSpace(text)
		}
	}
	delete(config, "defaultRole")
	delete(config, "adminGroupDN")
	discardSecretPresenceMarkers(config, []string{"bindPassword"})
	if !enabled {
		return removeEmptyConfigValues(config), nil
	}
	if stringValue(config["bindPassword"]) == "" {
		if value := stringValue(previous["bindPassword"]); value != "" {
			config["bindPassword"] = value
		}
	}
	if stringValue(config["host"]) == "" {
		return nil, fmt.Errorf("LDAP 服务器地址不能为空")
	}
	if stringValue(config["baseDN"]) == "" {
		return nil, fmt.Errorf("Base DN 不能为空")
	}
	if stringValue(config["userFilter"]) == "" {
		return nil, fmt.Errorf("用户过滤器不能为空")
	}
	if stringValue(config["bindDN"]) == "" {
		return nil, fmt.Errorf("绑定 DN 不能为空")
	}
	if stringValue(config["bindPassword"]) == "" {
		return nil, fmt.Errorf("绑定密码不能为空")
	}
	if boolValue(config["useTLS"]) && boolValue(config["startTLS"]) {
		return nil, fmt.Errorf("LDAPS 与 StartTLS 不能同时启用")
	}
	if numberValue(config["port"]) <= 0 {
		return nil, fmt.Errorf("端口不能为空")
	}
	return removeEmptyConfigValues(config), nil
}

func redactAuthProviders(items []domain.AuthProvider) []domain.AuthProvider {
	redacted := make([]domain.AuthProvider, len(items))
	for index, item := range items {
		redacted[index] = redactAuthProvider(item)
	}
	return redacted
}

func redactAuthProvider(item domain.AuthProvider) domain.AuthProvider {
	item.Config = redactConfigSecrets(item.Config, []string{"bindPassword"})
	return item
}
