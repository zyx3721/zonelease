package router

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
	authsvc "zonelease/backend/internal/service/auth"
)

const (
	wecomProviderID        = "wecom"
	wecomStatePurposeLogin = "login"
	wecomStatePurposeBind  = "bind"
	wecomCallbackPath      = "/api/auth/wecom/callback"
	wecomMaxRequestBodyKB  = 4
)

type wecomExchangeRequest struct {
	Ticket string `json:"ticket"`
}

type wecomBindRequest struct {
	Code   string `json:"code,omitempty"`
	State  string `json:"state,omitempty"`
	Ticket string `json:"ticket,omitempty"`
}

type wecomAuthorizeURLResponse struct {
	URL string `json:"url"`
}

type wecomBindingResponse struct {
	Bound       bool   `json:"bound"`
	WecomUserid string `json:"wecomUserid,omitempty"`
}

// wecomAuthorize GET /api/auth/wecom/authorize
// 登录入口：直连模式签发防伪 state 后跳企微授权页；统一认证中心模式跳认证中心 /login。
// 未启用或配置不完整时 302 回前端登录页并携带 wecomError，避免浏览器展示裸 JSON 错误。
func (r *Router) wecomAuthorize(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.logger.Warn("Wecom authorize unavailable", "error", err)
		r.redirectWecomError(w, req, cfg, wecomErrorFromSetup(err))
		return
	}
	var authorizeURL string
	if cfg.Mode == authsvc.WecomModeDirect {
		state, err := r.signWecomState(wecomStatePurposeLogin, "", req.Context())
		if err != nil {
			r.logger.Error("Sign wecom state failed", "error", err)
			r.redirectWecomError(w, req, cfg, wecomErrorLoginFailed)
			return
		}
		authorizeURL = client.AuthorizeURL(wecomExternalBase(req, cfg.RedirectPrefix)+wecomCallbackPath, state)
	} else {
		authorizeURL = client.AuthorizeURL("", "")
	}
	http.Redirect(w, req, authorizeURL, http.StatusFound)
}

// wecomCallback GET /api/auth/wecom/callback
// 直连模式校验 state 并用 code 换身份；认证中心模式（callback_path 配到后端的接法）用 ticket 调 verify 换身份。
// 成功后建立平台会话并签发一次性登录票据，重定向回前端登录页完成交换。
func (r *Router) wecomCallback(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.logger.Warn("Wecom provider unavailable on callback", "error", err)
		r.redirectWecomError(w, req, cfg, wecomErrorFromSetup(err))
		return
	}
	identity, err := r.resolveWecomIdentity(req, client, cfg)
	if err != nil {
		r.logger.Warn("Wecom identity resolve failed", "error", err)
		r.redirectWecomError(w, req, cfg, wecomErrorFromResolve(err))
		return
	}
	session, err := r.auth.LoginByWecom(req.Context(), wecomProviderID, identity)
	if err != nil {
		if errors.Is(err, authsvc.ErrWecomNotBound) {
			r.redirectWecomError(w, req, cfg, wecomErrorNotBound)
			return
		}
		if errors.Is(err, authsvc.ErrUserNotProvisioned) {
			r.redirectWecomError(w, req, cfg, wecomErrorNotProvisioned)
			return
		}
		r.logger.Error("Wecom login failed", "error", err)
		r.redirectWecomError(w, req, cfg, wecomErrorLoginFailed)
		return
	}
	ticket, err := r.auth.IssueWecomLoginTicket(session.Token)
	if err != nil {
		r.logger.Error("Issue wecom login ticket failed", "error", err)
		r.redirectWecomError(w, req, cfg, wecomErrorLoginFailed)
		return
	}
	r.store.WriteAudit(req.Context(), session.User.ID, session.User.Username, "User login", session.User.Username, "System", "success", auditMetadata(map[string]any{
		"username":    session.User.Username,
		"provider":    wecomProviderID,
		"wecomUserid": identity.Userid,
	}), repository.ClientIP(req))
	http.Redirect(w, req, wecomExternalBase(req, cfg.RedirectPrefix)+"/login?wecomTicket="+url.QueryEscape(ticket), http.StatusFound)
}

// wecomLogin POST /api/auth/wecom/login
// 认证中心回调路径配置为前端登录页时，前端持认证中心 ticket 调用本接口直接换取会话。
func (r *Router) wecomLogin(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.writeWecomSetupError(w, err)
		return
	}
	if cfg.Mode != authsvc.WecomModeCenter {
		writeError(w, http.StatusBadRequest, "invalid_mode", "当前并非统一认证中心模式，请使用扫码登录入口")
		return
	}
	var body wecomExchangeRequest
	if !decodeWithLimit(w, req, &body, wecomMaxRequestBodyKB) {
		return
	}
	identity, err := client.Exchange(req.Context(), body.Ticket)
	if err != nil {
		r.logger.Warn("Wecom center ticket login failed", "error", err)
		writeError(w, http.StatusUnauthorized, "invalid_ticket", wecomBindFailureMessage(err))
		return
	}
	session, err := r.auth.LoginByWecom(req.Context(), wecomProviderID, identity)
	if err != nil {
		if errors.Is(err, authsvc.ErrWecomNotBound) {
			writeError(w, http.StatusUnauthorized, "user_not_bound", "该企业微信账号尚未绑定平台用户，请先登录后在用户菜单绑定企业微信")
			return
		}
		if errors.Is(err, authsvc.ErrUserNotProvisioned) {
			writeError(w, http.StatusUnauthorized, "user_not_provisioned", "该用户已被禁用，请联系管理员")
			return
		}
		r.logger.Error("Wecom login failed", "error", err)
		writeError(w, http.StatusInternalServerError, "login_failed", "企业微信登录失败，请稍后重试")
		return
	}
	r.store.WriteAudit(req.Context(), session.User.ID, session.User.Username, "User login", session.User.Username, "System", "success", auditMetadata(map[string]any{
		"username":    session.User.Username,
		"provider":    wecomProviderID,
		"wecomUserid": identity.Userid,
	}), repository.ClientIP(req))
	writeJSON(w, http.StatusOK, session)
}

// wecomExchange POST /api/auth/wecom/exchange
// 前端持一次性登录票据换取正式会话；票据取出即删，60 秒有效。
func (r *Router) wecomExchange(w http.ResponseWriter, req *http.Request) {
	var body wecomExchangeRequest
	if !decodeWithLimit(w, req, &body, wecomMaxRequestBodyKB) {
		return
	}
	token, ok := r.auth.ConsumeWecomLoginTicket(body.Ticket)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_ticket", "登录凭证无效或已过期，请重新发起企业微信登录")
		return
	}
	session, err := r.auth.Validate(req.Context(), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_session", "会话已失效，请重新发起企业微信登录")
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// wecomBindURL GET /api/auth/wecom/bind-url
// 为当前登录用户签发绑定授权地址：直连模式 state 携带用途与用户 ID，回跳前端登录页；认证中心模式返回认证中心登录页。
func (r *Router) wecomBindURL(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.writeWecomSetupError(w, err)
		return
	}
	user := currentUser(req)
	var authorizeURL string
	if cfg.Mode == authsvc.WecomModeDirect {
		state, err := r.signWecomState(wecomStatePurposeBind, user.ID, req.Context())
		if err != nil {
			r.logger.Error("Sign wecom bind state failed", "error", err)
			writeError(w, http.StatusInternalServerError, "wecom_state_failed", "生成绑定状态失败")
			return
		}
		authorizeURL = client.AuthorizeURL(wecomExternalBase(req, cfg.RedirectPrefix)+"/login", state)
	} else {
		authorizeURL = client.AuthorizeURL("", "")
	}
	writeJSON(w, http.StatusOK, wecomAuthorizeURLResponse{URL: authorizeURL})
}

// wecomBind POST /api/auth/wecom/bind
// 直连模式校验绑定 state（用途与用户归属）后用 code 换 userid；认证中心模式用 ticket 换 userid。
// 绑定冲突返回 409，绑定 state 与当前用户不匹配返回 403。
func (r *Router) wecomBind(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.writeWecomSetupError(w, err)
		return
	}
	user := currentUser(req)
	var body wecomBindRequest
	if !decodeWithLimit(w, req, &body, wecomMaxRequestBodyKB) {
		return
	}
	var identity authsvc.WecomIdentity
	if cfg.Mode == authsvc.WecomModeDirect {
		claims, status := verifyWecomStateAt(body.State, r.cfg.Auth.SessionSecret, time.Now())
		if status != wecomStateValid || claims.Purpose != wecomStatePurposeBind {
			writeError(w, http.StatusBadRequest, "invalid_state", "绑定状态无效或已过期，请重新发起绑定")
			return
		}
		if claims.UserID != "" && claims.UserID != user.ID {
			writeError(w, http.StatusForbidden, "state_user_mismatch", "绑定状态与当前用户不匹配，请重新发起绑定")
			return
		}
		identity, err = client.Exchange(req.Context(), body.Code)
	} else {
		if strings.TrimSpace(body.Ticket) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "绑定票据不能为空")
			return
		}
		identity, err = client.Exchange(req.Context(), body.Ticket)
	}
	if err != nil {
		r.logger.Warn("Wecom bind identity resolve failed", "error", err)
		writeError(w, http.StatusBadGateway, "wecom_bind_failed", wecomBindFailureMessage(err))
		return
	}
	bound, err := r.auth.BindWecom(req.Context(), user.ID, identity.Userid)
	if err != nil {
		if errors.Is(err, authsvc.ErrWecomConflict) {
			writeError(w, http.StatusConflict, "wecom_bind_conflict", "该企业微信账号已绑定其他用户")
			return
		}
		r.logger.Error("Wecom bind failed", "error", err)
		writeError(w, http.StatusInternalServerError, "wecom_bind_failed", "绑定企业微信失败")
		return
	}
	r.writeAudit(req, "Bound wecom", user.Username, "System", "success", map[string]any{
		"username":    user.Username,
		"wecomUserid": bound,
	})
	writeJSON(w, http.StatusOK, wecomBindingResponse{Bound: true, WecomUserid: bound})
}

// wecomUnbind POST /api/auth/wecom/unbind
// 解除当前用户的企业微信绑定；未绑定时幂等返回 bound=false。
func (r *Router) wecomUnbind(w http.ResponseWriter, req *http.Request) {
	user := currentUser(req)
	previousUserid, wasBound, err := r.auth.WecomBindingOf(req.Context(), user.ID)
	if err != nil {
		r.logger.Error("Read wecom binding failed", "error", err)
		writeError(w, http.StatusInternalServerError, "wecom_unbind_failed", "解绑企业微信失败")
		return
	}
	removed, err := r.auth.UnbindWecom(req.Context(), user.ID)
	if err != nil {
		r.logger.Error("Wecom unbind failed", "error", err)
		writeError(w, http.StatusInternalServerError, "wecom_unbind_failed", "解绑企业微信失败")
		return
	}
	if removed {
		metadata := map[string]any{"username": user.Username}
		if wasBound {
			metadata["wecomUserid"] = previousUserid
		}
		r.writeAudit(req, "Unbound wecom", user.Username, "System", "success", metadata)
	}
	writeJSON(w, http.StatusOK, wecomBindingResponse{Bound: false})
}

func (r *Router) enabledWecomClient(req *http.Request) (authsvc.WecomClient, authsvc.WecomConfig, error) {
	provider, err := r.store.GetAuthProvider(req.Context(), wecomProviderID)
	if err != nil {
		return nil, authsvc.WecomConfig{}, authsvc.ErrWecomNotConfigured
	}
	if !provider.Enabled {
		return nil, authsvc.WecomConfig{}, authsvc.ErrWecomNotConfigured
	}
	return authsvc.NewWecomClient(provider)
}

func (r *Router) resolveWecomIdentity(req *http.Request, client authsvc.WecomClient, cfg authsvc.WecomConfig) (authsvc.WecomIdentity, error) {
	query := req.URL.Query()
	if cfg.Mode == authsvc.WecomModeDirect {
		state := query.Get("state")
		claims, status := verifyWecomStateAt(state, r.cfg.Auth.SessionSecret, time.Now())
		switch status {
		case wecomStateInvalid:
			return authsvc.WecomIdentity{}, errors.New("wecom state invalid")
		case wecomStateExpired:
			return authsvc.WecomIdentity{}, errors.New("wecom state expired")
		}
		if claims.Purpose != wecomStatePurposeLogin {
			return authsvc.WecomIdentity{}, errors.New("wecom state invalid")
		}
		return client.Exchange(req.Context(), query.Get("code"))
	}
	ticket := strings.TrimSpace(query.Get("ticket"))
	if ticket == "" {
		return authsvc.WecomIdentity{}, errors.New("wecom ticket missing")
	}
	return client.Exchange(req.Context(), ticket)
}

func (r *Router) writeWecomSetupError(w http.ResponseWriter, err error) {
	if errors.Is(err, authsvc.ErrWecomNotConfigured) {
		writeError(w, http.StatusNotFound, "wecom_not_enabled", "企业微信登录未启用")
		return
	}
	writeError(w, http.StatusServiceUnavailable, "wecom_config_invalid", "企业微信登录配置不完整，请检查认证配置")
}

func (r *Router) redirectWecomError(w http.ResponseWriter, req *http.Request, cfg authsvc.WecomConfig, code string) {
	prefix := cfg.RedirectPrefix
	http.Redirect(w, req, wecomExternalBase(req, prefix)+"/login?wecomError="+url.QueryEscape(code), http.StatusFound)
}

const (
	wecomErrorInvalidState   = "state_invalid"
	wecomErrorExpiredState   = "state_expired"
	wecomErrorWecomFailed    = "wecom_error"
	wecomErrorCenterFailed   = "center_error"
	wecomErrorNotBound       = "user_not_bound"
	wecomErrorNotProvisioned = "user_not_provisioned"
	wecomErrorLoginFailed    = "login_failed"
	wecomErrorNotEnabled     = "wecom_not_enabled"
	wecomErrorConfigInvalid  = "wecom_config_invalid"
)

func wecomErrorFromSetup(err error) string {
	if errors.Is(err, authsvc.ErrWecomNotConfigured) {
		return wecomErrorNotEnabled
	}
	return wecomErrorConfigInvalid
}

func wecomErrorFromResolve(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "state expired"):
		return wecomErrorExpiredState
	case strings.Contains(message, "state invalid"):
		return wecomErrorInvalidState
	case strings.Contains(message, "auth center"):
		return wecomErrorCenterFailed
	default:
		return wecomErrorWecomFailed
	}
}

func wecomBindFailureMessage(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "state expired"):
		return "绑定状态已过期，请重新发起绑定"
	case strings.Contains(message, "auth center"):
		return "统一认证中心连接失败，请稍后重试"
	default:
		return "企业微信身份获取失败，请稍后重试"
	}
}

// wecomExternalBase 推断外部访问基址：优先使用配置的回调前缀，其次按反代头或请求 Host 推断。
func wecomExternalBase(req *http.Request, prefix string) string {
	if prefix = strings.TrimRight(strings.TrimSpace(prefix), "/"); prefix != "" {
		if strings.HasPrefix(prefix, "http://") || strings.HasPrefix(prefix, "https://") {
			return prefix
		}
	}
	scheme := req.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if req.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := req.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = req.Host
	}
	return scheme + "://" + host
}

// wecomStateClaims 防伪 state 载荷：用途 + 绑定发起用户 + 随机数 + 过期时间，JWT_SECRET 签名防伪造。
type wecomStateClaims struct {
	Purpose   string `json:"p"`
	UserID    string `json:"u,omitempty"`
	Nonce     string `json:"n"`
	ExpiresAt int64  `json:"e"`
}

// wecomStateTTL 读取基础配置中的企业微信扫码有效期，缺省或越界时回落默认值。
func (r *Router) wecomStateTTL(ctx context.Context) time.Duration {
	defaults := time.Duration(domain.DefaultSystemBaseConfig().WecomStateTtlMinutes) * time.Minute
	if r.store == nil {
		return defaults
	}
	config, err := r.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return defaults
	}
	minutes := domain.NormalizeSystemBaseConfig(config).WecomStateTtlMinutes
	return time.Duration(minutes) * time.Minute
}

func (r *Router) signWecomState(purpose, userID string, ctx context.Context) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	claims := wecomStateClaims{
		Purpose:   purpose,
		UserID:    userID,
		Nonce:     hex.EncodeToString(nonce),
		ExpiresAt: time.Now().Add(r.wecomStateTTL(ctx)).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + wecomStateSignature(r.cfg.Auth.SessionSecret, encoded), nil
}

type wecomStateStatus int

const (
	wecomStateInvalid wecomStateStatus = iota
	wecomStateExpired
	wecomStateValid
)

func verifyWecomStateAt(state, secret string, now time.Time) (wecomStateClaims, wecomStateStatus) {
	var claims wecomStateClaims
	encoded, signature, found := strings.Cut(state, ".")
	if !found || encoded == "" || signature == "" {
		return claims, wecomStateInvalid
	}
	if subtle.ConstantTimeCompare([]byte(signature), []byte(wecomStateSignature(secret, encoded))) != 1 {
		return claims, wecomStateInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return claims, wecomStateInvalid
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return wecomStateClaims{}, wecomStateInvalid
	}
	if claims.Purpose == "" || claims.Nonce == "" {
		return wecomStateClaims{}, wecomStateInvalid
	}
	if now.Unix() >= claims.ExpiresAt {
		return wecomStateClaims{}, wecomStateExpired
	}
	return claims, wecomStateValid
}

func wecomStateSignature(secret, encoded string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	return hex.EncodeToString(mac.Sum(nil))
}

// decodeWithLimit 复用 decode 的错误处理约定，但以 KB 为单位收紧请求体上限。
func decodeWithLimit(w http.ResponseWriter, req *http.Request, dst any, limitKB int) bool {
	defer req.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, int64(limitKB)<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是合法 JSON")
		return false
	}
	return true
}
