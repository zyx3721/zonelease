package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

var (
	ErrInvalidCredentials   = errors.New("invalid username or password")
	ErrInvalidSession       = errors.New("invalid or expired session")
	ErrInvalidResetToken    = errors.New("invalid password reset token")
	ErrInvalidResetCaptcha  = errors.New("invalid password reset captcha")
	ErrResetUnavailable     = errors.New("password reset unavailable")
	ErrResetChannelMissing  = errors.New("password reset channel missing")
	ErrResetCodeMismatch    = errors.New("invalid password reset code")
	ErrOldPasswordMismatch  = errors.New("old password mismatch")
	ErrResetEmailMismatch   = errors.New("password reset email mismatch")
	ErrUserNotProvisioned   = errors.New("external user is not provisioned")
	ErrResetCodeCooldown    = errors.New("password reset code cooldown")
	ErrResetCodeRateLimited = errors.New("password reset code rate limited")
)

type ResetCodeCooldownError struct {
	RemainingSeconds int
}

func (e ResetCodeCooldownError) Error() string {
	return ErrResetCodeCooldown.Error()
}

func (e ResetCodeCooldownError) Is(target error) bool {
	return target == ErrResetCodeCooldown
}

type Store interface {
	FindUserByUsername(ctx context.Context, username string) (domain.User, string, error)
	FindUserByID(ctx context.Context, id string) (domain.User, error)
	GetAuthProvider(ctx context.Context, id string) (domain.AuthProvider, error)
	RecordUserLogin(ctx context.Context, userID string) error
	UpdateUserPassword(ctx context.Context, userID, passwordHash string) error
	CreateSession(ctx context.Context, token string, userID string, expiresAt time.Time) error
	FindSession(ctx context.Context, token string) (domain.User, time.Time, time.Time, error)
	TouchSession(ctx context.Context, token string) error
	DeleteSession(ctx context.Context, token string) error
	DeleteUserSessions(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context) error
	CreatePasswordResetRequest(ctx context.Context, token, userID string, expiresAt time.Time) error
	SetPasswordResetCode(ctx context.Context, token, codeHash, channel string, expiresAt time.Time) error
	FindPasswordResetRequest(ctx context.Context, token string) (repository.PasswordResetRequest, error)
	MarkPasswordResetUsed(ctx context.Context, token string) error
	LatestRecentPasswordResetCodeSentAt(ctx context.Context, userID string, since time.Time) (time.Time, bool, error)
	CountRecentPasswordResetCodes(ctx context.Context, userID string, since time.Time) (int, error)
	GetSystemBaseConfig(ctx context.Context) (domain.SystemBaseConfig, error)
	GetPasswordResetNotificationChannel(ctx context.Context) (domain.NotificationChannel, error)
}

type ResetNotifier interface {
	SendPasswordResetCode(ctx context.Context, to, code string) error
	SendPasswordReset(ctx context.Context, to, code string, expiresAt time.Time) error
}

type Config struct {
	SessionSecret         string
	SessionTTL            time.Duration
	SessionIdleTTL        time.Duration
	ResetCodeTTL          time.Duration
	ResetCaptchaTTL       time.Duration
	ResetVerificationTTL  time.Duration
	ResetSendCooldownSecs int
	ResetRateLimitSpan    time.Duration
	ResetRateLimitMax     int
}

type Service struct {
	store    Store
	cfg      Config
	notifier ResetNotifier
	now      func() time.Time
}

func New(store Store, cfg Config) *Service {
	return &Service{store: store, cfg: cfg, now: time.Now}
}

func (s *Service) SetNotifier(notifier ResetNotifier) {
	s.notifier = notifier
}

func (s *Service) Login(ctx context.Context, username, password string) (domain.Session, error) {
	user, passwordHash, err := s.store.FindUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil || user.Disabled {
		return domain.Session{}, ErrInvalidCredentials
	}
	if err := repository.VerifyPassword(passwordHash, password); err != nil {
		return domain.Session{}, ErrInvalidCredentials
	}
	return s.createSession(ctx, user)
}

func (s *Service) LoginWithProvider(ctx context.Context, providerID, username, password string) (domain.Session, error) {
	if providerID == "" || providerID == "local" {
		return s.Login(ctx, username, password)
	}
	provider, err := s.store.GetAuthProvider(ctx, providerID)
	if err != nil || !provider.Enabled || provider.Type != "ldap" {
		return domain.Session{}, ErrInvalidCredentials
	}
	cfg, err := decodeLDAPConfig(provider.Config)
	if err != nil {
		return domain.Session{}, err
	}
	user, err := authenticateLDAP(ctx, cfg, username, password)
	if err != nil {
		return domain.Session{}, ErrInvalidCredentials
	}
	stored, _, err := s.store.FindUserByUsername(ctx, user.Username)
	if err != nil || stored.Disabled {
		return domain.Session{}, ErrUserNotProvisioned
	}
	return s.createSession(ctx, stored)
}

func TestLDAPProvider(ctx context.Context, provider domain.AuthProvider) (LDAPTestResult, error) {
	cfg, err := decodeLDAPConfig(provider.Config)
	if err != nil {
		return LDAPTestResult{}, err
	}
	conn, err := dialLDAP(ctx, cfg)
	if err != nil {
		return LDAPTestResult{}, err
	}
	defer conn.Close()
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return LDAPTestResult{}, err
		}
	}
	matchedUsers, err := countLDAPTestUsers(conn, cfg)
	if err != nil {
		return LDAPTestResult{}, err
	}
	return LDAPTestResult{MatchedUsers: matchedUsers}, nil
}

func LDAPUserMessage(err error) string {
	if err == nil {
		return ""
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return "LDAP TLS 证书不受信任，请导入可信证书或勾选跳过证书校验"
	}
	var hostError x509.HostnameError
	if errors.As(err, &hostError) {
		return "LDAP TLS 证书域名与服务器地址不匹配，请检查证书或勾选跳过证书校验"
	}
	var certInvalidError x509.CertificateInvalidError
	if errors.As(err, &certInvalidError) {
		return "LDAP TLS 证书无效或已过期，请检查证书配置"
	}
	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return "LDAP 服务连接超时，请检查服务器地址、端口和网络连通性"
	}
	if errors.Is(err, io.EOF) {
		return "LDAP 服务提前断开连接，请确认端口协议是否匹配"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "connection refused") {
		return "LDAP 服务拒绝连接，请检查端口是否开放"
	}
	if strings.Contains(message, "connection reset") {
		return "LDAP 连接被重置，请确认端口协议和 TLS 配置是否匹配"
	}
	if strings.Contains(message, "first record does not look like a tls handshake") {
		return "当前端口不是 LDAPS 服务，请改用 389 或关闭 LDAPS"
	}
	if strings.Contains(message, "unsupported protocol version") || strings.Contains(message, "protocol version 301") {
		return "LDAP TLS 版本过低，请启用 TLS 1.2+"
	}
	if strings.Contains(message, "start tls") || strings.Contains(message, "starttls") {
		return "LDAP 服务不支持 StartTLS 或 StartTLS 握手失败，请检查服务端配置"
	}
	if strings.Contains(message, "invalid credentials") {
		return "LDAP 绑定账号或密码不正确"
	}
	var filterErr ldapFilterSearchError
	if errors.As(err, &filterErr) && strings.Contains(message, "filter compile error") {
		return filterErr.Source + "格式不正确，请填写完整 LDAP 过滤器"
	}
	if strings.Contains(message, "filter compile error") {
		return "LDAP 过滤器格式不正确，请检查用户过滤器或用户组过滤器"
	}
	return "认证服务连接测试失败：" + err.Error()
}

func (s *Service) Validate(ctx context.Context, token string) (domain.Session, error) {
	if strings.TrimSpace(token) == "" {
		return domain.Session{}, ErrInvalidSession
	}
	_ = s.store.DeleteExpiredSessions(ctx)
	user, expiresAt, lastSeenAt, err := s.store.FindSession(ctx, token)
	now := s.now()
	if err != nil || !expiresAt.After(now) || !lastSeenAt.Add(s.cfg.SessionIdleTTL).After(now) || user.Disabled {
		return domain.Session{}, ErrInvalidSession
	}
	if now.Sub(lastSeenAt) >= 5*time.Minute {
		if err := s.store.TouchSession(ctx, token); err != nil {
			return domain.Session{}, err
		}
		lastSeenAt = now
	}
	return domain.Session{Token: token, ExpiresAt: expiresAt, LastSeenAt: lastSeenAt, User: user}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, token)
}

func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	user, err := s.store.FindUserByID(ctx, userID)
	if err != nil || user.Disabled {
		return ErrInvalidSession
	}
	_, passwordHash, err := s.store.FindUserByUsername(ctx, user.Username)
	if err != nil {
		return ErrInvalidSession
	}
	if err := repository.VerifyPassword(passwordHash, oldPassword); err != nil {
		return ErrOldPasswordMismatch
	}
	hash, err := repository.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.store.UpdateUserPassword(ctx, user.ID, hash)
}

func (s *Service) createSession(ctx context.Context, user domain.User) (domain.Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return domain.Session{}, err
	}
	now := s.now()
	expiresAt := now.Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(ctx, token, user.ID, expiresAt); err != nil {
		return domain.Session{}, err
	}
	if err := s.store.RecordUserLogin(ctx, user.ID); err != nil {
		return domain.Session{}, err
	}
	return domain.Session{Token: token, ExpiresAt: expiresAt, LastSeenAt: now, User: user}, nil
}

type LDAPConfig struct {
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	BaseDN               string `json:"baseDN"`
	UserFilter           string `json:"userFilter"`
	BindDN               string `json:"bindDN"`
	BindPassword         string `json:"bindPassword"`
	UseTLS               bool   `json:"useTLS"`
	StartTLS             bool   `json:"startTLS"`
	InsecureSkipVerify   bool   `json:"insecureSkipVerify"`
	UsernameAttribute    string `json:"usernameAttribute"`
	DisplayNameAttribute string `json:"displayNameAttribute"`
	EmailAttribute       string `json:"emailAttribute"`
	TimeoutSeconds       int    `json:"timeoutSeconds"`
	GroupFilter          string `json:"groupFilter"`
}

type LDAPUser struct {
	Username    string
	DisplayName string
}

type LDAPTestResult struct {
	MatchedUsers int
}

type ldapFilterSearchError struct {
	Source string
	Err    error
}

func (err ldapFilterSearchError) Error() string {
	return err.Source + " search failed: " + err.Err.Error()
}

func (err ldapFilterSearchError) Unwrap() error {
	return err.Err
}

func decodeLDAPConfig(data []byte) (LDAPConfig, error) {
	var cfg LDAPConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return LDAPConfig{}, err
		}
	}
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.BaseDN = strings.TrimSpace(cfg.BaseDN)
	cfg.UserFilter = strings.TrimSpace(cfg.UserFilter)
	cfg.BindDN = strings.TrimSpace(cfg.BindDN)
	cfg.UsernameAttribute = strings.TrimSpace(cfg.UsernameAttribute)
	cfg.DisplayNameAttribute = strings.TrimSpace(cfg.DisplayNameAttribute)
	cfg.EmailAttribute = strings.TrimSpace(cfg.EmailAttribute)
	cfg.GroupFilter = strings.TrimSpace(cfg.GroupFilter)
	if cfg.Port == 0 {
		if cfg.UseTLS {
			cfg.Port = 636
		} else {
			cfg.Port = 389
		}
	}
	if cfg.UseTLS && cfg.StartTLS {
		return LDAPConfig{}, fmt.Errorf("LDAPS and StartTLS cannot both be enabled")
	}
	if cfg.UserFilter == "" {
		cfg.UserFilter = "(sAMAccountName={username})"
	}
	if cfg.UsernameAttribute == "" {
		cfg.UsernameAttribute = "sAMAccountName"
	}
	if cfg.DisplayNameAttribute == "" {
		cfg.DisplayNameAttribute = "displayName"
	}
	if cfg.EmailAttribute == "" {
		cfg.EmailAttribute = "mail"
	}
	if cfg.TimeoutSeconds <= 0 || cfg.TimeoutSeconds > 30 {
		cfg.TimeoutSeconds = 8
	}
	if cfg.Host == "" || cfg.BaseDN == "" {
		return LDAPConfig{}, fmt.Errorf("LDAP host and base DN are required")
	}
	return cfg, nil
}

func authenticateLDAP(ctx context.Context, cfg LDAPConfig, username, password string) (LDAPUser, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return LDAPUser{}, ErrInvalidCredentials
	}
	conn, err := dialLDAP(ctx, cfg)
	if err != nil {
		return LDAPUser{}, err
	}
	defer conn.Close()
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return LDAPUser{}, err
		}
	}
	filter := ldapLoginUserFilter(cfg, username)
	search := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1,
		cfg.TimeoutSeconds,
		false,
		filter,
		[]string{cfg.UsernameAttribute, cfg.DisplayNameAttribute, cfg.EmailAttribute, "dn"},
		nil,
	)
	result, err := conn.Search(search)
	if err != nil || len(result.Entries) != 1 {
		return LDAPUser{}, ErrInvalidCredentials
	}
	entry := result.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return LDAPUser{}, ErrInvalidCredentials
	}
	resolvedUsername := entry.GetAttributeValue(cfg.UsernameAttribute)
	if strings.TrimSpace(resolvedUsername) == "" {
		resolvedUsername = username
	}
	displayName := entry.GetAttributeValue(cfg.DisplayNameAttribute)
	if strings.TrimSpace(displayName) == "" {
		displayName = resolvedUsername
	}
	return LDAPUser{Username: resolvedUsername, DisplayName: displayName}, nil
}

func ldapLoginUserFilter(cfg LDAPConfig, username string) string {
	userFilter := strings.ReplaceAll(cfg.UserFilter, "{username}", ldap.EscapeFilter(username))
	groupFilter := ldapNormalizedGroupFilter(cfg)
	if groupFilter == "" {
		return userFilter
	}
	return "(&" + userFilter + groupFilter + ")"
}

func ldapNormalizedGroupFilter(cfg LDAPConfig) string {
	groupFilter := strings.TrimSpace(cfg.GroupFilter)
	if groupFilter == "" {
		return ""
	}
	if strings.HasPrefix(groupFilter, "(") {
		return groupFilter
	}
	return "(memberOf=" + ldap.EscapeFilter(groupFilter) + ")"
}

func ldapTestUserFilter(cfg LDAPConfig) (string, string) {
	if cfg.GroupFilter != "" {
		return ldapNormalizedGroupFilter(cfg), "用户组过滤器"
	}
	return strings.ReplaceAll(cfg.UserFilter, "{username}", "*"), "用户过滤器"
}

func countLDAPTestUsers(conn *ldap.Conn, cfg LDAPConfig) (int, error) {
	filter, source := ldapTestUserFilter(cfg)
	search := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		cfg.TimeoutSeconds,
		false,
		filter,
		[]string{"dn"},
		nil,
	)
	result, err := conn.SearchWithPaging(search, 500)
	if err != nil {
		return 0, ldapFilterSearchError{Source: source, Err: err}
	}
	return len(result.Entries), nil
}

func dialLDAP(ctx context.Context, cfg LDAPConfig) (*ldap.Conn, error) {
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	tlsConfig := &tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec
	var conn *ldap.Conn
	var err error
	if cfg.UseTLS {
		dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: tlsConfig}
		rawConn, dialErr := dialer.DialContext(ctx, "tcp", address)
		if dialErr != nil {
			return nil, dialErr
		}
		conn = ldap.NewConn(rawConn, true)
		conn.Start()
	} else {
		dialer := &net.Dialer{Timeout: timeout}
		rawConn, dialErr := dialer.DialContext(ctx, "tcp", address)
		if dialErr != nil {
			return nil, dialErr
		}
		conn = ldap.NewConn(rawConn, false)
		conn.Start()
		if cfg.StartTLS {
			err = conn.StartTLS(tlsConfig)
		}
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetTimeout(timeout)
	return conn, nil
}
