package router

import (
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

	"zonelease/backend/internal/repository"
	authsvc "zonelease/backend/internal/service/auth"
)

const (
	wecomProviderID         = "wecom"
	wecomStateTTL           = 5 * time.Minute
	wecomStatePurpose       = "zonelease-wecom-login"
	wecomCallbackPath       = "/api/auth/wecom/callback"
	wecomMaxRequestBodySize = 4 << 10
)

type wecomExchangeRequest struct {
	Ticket string `json:"ticket"`
}

// wecomAuthorize GET /api/auth/wecom/authorize
// 生成授权跳转：直连模式签发防伪 state 后跳企微授权页；统一认证中心模式直接跳认证中心 /login。
func (r *Router) wecomAuthorize(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.writeWecomSetupError(w, err)
		return
	}
	var authorizeURL string
	if cfg.Mode == authsvc.WecomModeDirect {
		state, err := r.signWecomState()
		if err != nil {
			r.logger.Error("Sign wecom state failed", "error", err)
			writeError(w, http.StatusInternalServerError, "wecom_state_failed", "生成登录状态失败")
			return
		}
		authorizeURL = client.AuthorizeURL(wecomExternalBase(req, cfg.RedirectPrefix)+wecomCallbackPath, state)
	} else {
		authorizeURL = client.AuthorizeURL("", "")
	}
	http.Redirect(w, req, authorizeURL, http.StatusFound)
}

// wecomCallback GET /api/auth/wecom/callback
// 直连模式校验 state 并用 code 换身份；认证中心模式用 ticket 调 verify 换身份。
// 成功后建立平台会话并签发一次性登录票据，重定向回前端登录页完成交换。
func (r *Router) wecomCallback(w http.ResponseWriter, req *http.Request) {
	client, cfg, err := r.enabledWecomClient(req)
	if err != nil {
		r.logger.Warn("Wecom provider unavailable on callback", "error", err)
		r.redirectWecomError(w, req, cfg, wecomErrorWecomFailed)
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

// wecomExchange POST /api/auth/wecom/exchange
// 前端持一次性登录票据换取正式会话；票据取出即删，60 秒有效。
func (r *Router) wecomExchange(w http.ResponseWriter, req *http.Request) {
	var body wecomExchangeRequest
	if !decodeWithLimit(w, req, &body, wecomMaxRequestBodySize) {
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
		switch verifyWecomState(state, r.cfg.Auth.SessionSecret) {
		case wecomStateInvalid:
			return authsvc.WecomIdentity{}, errors.New("wecom state invalid")
		case wecomStateExpired:
			return authsvc.WecomIdentity{}, errors.New("wecom state expired")
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
	wecomErrorNotProvisioned = "user_not_provisioned"
	wecomErrorLoginFailed    = "login_failed"
)

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

// wecomStateClaims 防伪 state 载荷：用途 + 随机数 + 过期时间，JWT_SECRET 签名防伪造。
type wecomStateClaims struct {
	Purpose   string `json:"p"`
	Nonce     string `json:"n"`
	ExpiresAt int64  `json:"e"`
}

func (r *Router) signWecomState() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	claims := wecomStateClaims{
		Purpose:   wecomStatePurpose,
		Nonce:     hex.EncodeToString(nonce),
		ExpiresAt: time.Now().Add(wecomStateTTL).Unix(),
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

func verifyWecomState(state, secret string) wecomStateStatus {
	return verifyWecomStateAt(state, secret, time.Now())
}

func verifyWecomStateAt(state, secret string, now time.Time) wecomStateStatus {
	encoded, signature, found := strings.Cut(state, ".")
	if !found || encoded == "" || signature == "" {
		return wecomStateInvalid
	}
	if subtle.ConstantTimeCompare([]byte(signature), []byte(wecomStateSignature(secret, encoded))) != 1 {
		return wecomStateInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return wecomStateInvalid
	}
	var claims wecomStateClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return wecomStateInvalid
	}
	if claims.Purpose != wecomStatePurpose || claims.Nonce == "" {
		return wecomStateInvalid
	}
	if now.Unix() >= claims.ExpiresAt {
		return wecomStateExpired
	}
	return wecomStateValid
}

func wecomStateSignature(secret, encoded string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	return hex.EncodeToString(mac.Sum(nil))
}

// decodeWithLimit 复用 decode 的错误处理约定，但允许覆盖请求体大小上限。
func decodeWithLimit(w http.ResponseWriter, req *http.Request, dst any, limit int64) bool {
	defer req.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是合法 JSON")
		return false
	}
	return true
}
