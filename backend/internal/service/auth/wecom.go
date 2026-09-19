package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"zonelease/backend/internal/domain"
)

const (
	WecomModeDirect = "direct"
	WecomModeCenter = "center"

	wecomQyapiBase = "https://qyapi.weixin.qq.com"
	wecomQRLogin   = "https://login.work.weixin.qq.com/wwlogin/sso/login"
)

var ErrWecomNotConfigured = errors.New("wecom provider is not configured")

// WecomConfig 认证配置（auth_providers.config JSONB）。
// Mode 为 direct 时使用企微直连字段；为 center 时使用统一认证中心字段。
type WecomConfig struct {
	Mode    string `json:"mode"`
	CorpID  string `json:"corpId"`
	AgentID int    `json:"agentId"`
	Secret  string `json:"secret"`

	AuthCenterURL string `json:"authCenterUrl"`
	AppID         string `json:"appId"`
	AppSecret     string `json:"appSecret"`

	RedirectPrefix string `json:"redirectPrefix"`
}

// WecomIdentity 企业微信换取到的用户身份。
type WecomIdentity struct {
	Userid string
	Name   string
}

type WecomClient interface {
	// Exchange 用直连 code 或认证中心 ticket 换取企业微信用户身份。
	Exchange(ctx context.Context, credential string) (WecomIdentity, error)
	// AuthorizeURL 拼接授权跳转地址；redirectURI 仅直连模式使用。
	AuthorizeURL(redirectURI, state string) string
}

func NewWecomClient(provider domain.AuthProvider) (WecomClient, WecomConfig, error) {
	cfg, err := decodeWecomConfig(provider.Config)
	if err != nil {
		return nil, WecomConfig{}, err
	}
	if cfg.Mode == WecomModeCenter {
		return &WecomCenterClient{cfg: cfg, httpc: &http.Client{Timeout: 10 * time.Second}}, cfg, nil
	}
	return &WecomDirectClient{
		cfg:   cfg,
		base:  wecomQyapiBase,
		httpc: &http.Client{Timeout: 10 * time.Second},
		now:   time.Now,
	}, cfg, nil
}

func decodeWecomConfig(data []byte) (WecomConfig, error) {
	var cfg WecomConfig
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return WecomConfig{}, err
		}
	}
	cfg.Mode = strings.TrimSpace(cfg.Mode)
	cfg.CorpID = strings.TrimSpace(cfg.CorpID)
	cfg.Secret = strings.TrimSpace(cfg.Secret)
	cfg.AuthCenterURL = strings.TrimRight(strings.TrimSpace(cfg.AuthCenterURL), "/")
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)
	cfg.RedirectPrefix = strings.TrimRight(strings.TrimSpace(cfg.RedirectPrefix), "/")
	if cfg.Mode == "" {
		cfg.Mode = WecomModeDirect
	}
	if cfg.Mode != WecomModeDirect && cfg.Mode != WecomModeCenter {
		return WecomConfig{}, fmt.Errorf("unknown wecom mode %q", cfg.Mode)
	}
	if cfg.Mode == WecomModeDirect {
		if cfg.CorpID == "" || cfg.Secret == "" || cfg.AgentID <= 0 {
			return WecomConfig{}, ErrWecomNotConfigured
		}
		return cfg, nil
	}
	if cfg.AuthCenterURL == "" || cfg.AppID == "" || cfg.AppSecret == "" {
		return WecomConfig{}, ErrWecomNotConfigured
	}
	return cfg, nil
}

// wecomVerifyRequest 统一认证中心 /api/verify 请求体。
type wecomVerifyRequest struct {
	App    string `json:"app"`
	Ticket string `json:"ticket"`
	TS     int64  `json:"ts"`
	Sign   string `json:"sign"`
}

// WecomDirectClient 企业微信直连客户端：gettoken 缓存 + getuserinfo，可选 user/get 补全姓名。
type WecomDirectClient struct {
	cfg   WecomConfig
	base  string
	httpc *http.Client
	now   func() time.Time

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func (c *WecomDirectClient) Exchange(ctx context.Context, credential string) (WecomIdentity, error) {
	token, err := c.token(ctx)
	if err != nil {
		return WecomIdentity{}, err
	}
	var resp struct {
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
		Userid  string `json:"userid"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/cgi-bin/auth/getuserinfo?access_token=%s&code=%s",
		c.base, url.QueryEscape(token), url.QueryEscape(credential)), &resp); err != nil {
		return WecomIdentity{}, err
	}
	if resp.Errcode != 0 {
		return WecomIdentity{}, fmt.Errorf("getuserinfo errcode=%d errmsg=%s", resp.Errcode, resp.Errmsg)
	}
	if resp.Userid == "" {
		return WecomIdentity{}, errors.New("getuserinfo returned empty userid")
	}
	return WecomIdentity{Userid: resp.Userid}, nil
}

// token 返回缓存的 access_token；到期前 5 分钟主动刷新，并发调用仅触发一次请求。
func (c *WecomDirectClient) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && c.now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}
	var resp struct {
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		c.base, url.QueryEscape(c.cfg.CorpID), url.QueryEscape(c.cfg.Secret)), &resp); err != nil {
		return "", err
	}
	if resp.Errcode != 0 {
		return "", fmt.Errorf("gettoken errcode=%d errmsg=%s", resp.Errcode, resp.Errmsg)
	}
	if resp.AccessToken == "" {
		return "", errors.New("gettoken returned empty access_token")
	}
	c.accessToken = resp.AccessToken
	c.tokenExpiry = c.now().Add(time.Duration(resp.ExpiresIn-300) * time.Second)
	return c.accessToken, nil
}

func (c *WecomDirectClient) AuthorizeURL(redirectURI, state string) string {
	query := url.Values{}
	query.Set("appid", c.cfg.CorpID)
	query.Set("agentid", strconv.Itoa(c.cfg.AgentID))
	query.Set("login_type", "CorpApp")
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	return wecomQRLogin + "?" + query.Encode()
}

func (c *WecomDirectClient) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	httpResp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("wecom api http %d: %s", httpResp.StatusCode, endpoint)
	}
	return json.NewDecoder(io.LimitReader(httpResp.Body, 1<<20)).Decode(out)
}

// WecomCenterClient 统一认证中心客户端：用 HMAC-SHA256 签名调用 /api/verify 一次性换取身份。
type WecomCenterClient struct {
	cfg   WecomConfig
	httpc *http.Client
}

func (c *WecomCenterClient) Exchange(ctx context.Context, credential string) (WecomIdentity, error) {
	ts := time.Now().Unix()
	// 与 wecom-auth-center /api/verify 的请求结构严格对齐，ts 必须是 JSON 数字
	body, err := json.Marshal(wecomVerifyRequest{
		App:    c.cfg.AppID,
		Ticket: credential,
		TS:     ts,
		Sign:   SignWecomTicket(c.cfg.AppSecret, c.cfg.AppID, credential, ts),
	})
	if err != nil {
		return WecomIdentity{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.AuthCenterURL+"/api/verify", strings.NewReader(string(body)))
	if err != nil {
		return WecomIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpResp, err := c.httpc.Do(req)
	if err != nil {
		return WecomIdentity{}, err
	}
	defer httpResp.Body.Close()
	var result struct {
		Userid string `json:"userid"`
		Name   string `json:"name"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(httpResp.Body, 4<<10)).Decode(&result); err != nil {
		return WecomIdentity{}, fmt.Errorf("auth center verify http %d: %w", httpResp.StatusCode, err)
	}
	if httpResp.StatusCode != http.StatusOK {
		if result.Error != "" {
			return WecomIdentity{}, fmt.Errorf("auth center verify rejected: %s", result.Error)
		}
		return WecomIdentity{}, fmt.Errorf("auth center verify http %d", httpResp.StatusCode)
	}
	if result.Userid == "" {
		return WecomIdentity{}, errors.New("auth center verify returned empty userid")
	}
	return WecomIdentity{Userid: result.Userid, Name: result.Name}, nil
}

// AuthorizeURL 跳转统一认证中心登录入口；redirectURI 仅用于构造认证中心 /login 的回退地址，当前固定为空。
func (c *WecomCenterClient) AuthorizeURL(redirectURI, state string) string {
	query := url.Values{}
	query.Set("app", c.cfg.AppID)
	return c.cfg.AuthCenterURL + "/login?" + query.Encode()
}

// SignWecomTicket 统一认证中心 verify 签名算法：
// sign = hex(HMAC-SHA256(key=app_secret, msg=app+"\n"+ticket+"\n"+ts))。
func SignWecomTicket(secret, app, ticket string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s\n%s\n%s", app, ticket, strconv.FormatInt(ts, 10))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWecomTicketSignature 常数时间比较签名，避免时序侧信道。
func VerifyWecomTicketSignature(secret, app, ticket, sign string, ts int64) bool {
	expected := SignWecomTicket(secret, app, ticket, ts)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sign)) == 1
}
