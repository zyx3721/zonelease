package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"zonelease/backend/internal/domain"
)

func domainUserForTest(username string) domain.User {
	return domain.User{ID: "user-" + username, Username: username, DisplayName: username, Role: "admin"}
}

func TestDecodeWecomConfigDefaults(t *testing.T) {
	cfg, err := decodeWecomConfig([]byte(`{"corpId":"ww123","agentId":1000002,"secret":"s3cret"}`))
	if err != nil {
		t.Fatalf("decodeWecomConfig returned error: %v", err)
	}
	if cfg.Mode != WecomModeDirect {
		t.Fatalf("mode = %q, want direct", cfg.Mode)
	}
}

func TestDecodeWecomConfigCenterTrimsTrailingSlash(t *testing.T) {
	cfg, err := decodeWecomConfig([]byte(`{"mode":"center","authCenterUrl":"https://auth.example.com/","appId":"zonelease","appSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("decodeWecomConfig returned error: %v", err)
	}
	if cfg.AuthCenterURL != "https://auth.example.com" {
		t.Fatalf("authCenterUrl = %q, want without trailing slash", cfg.AuthCenterURL)
	}
}

func TestDecodeWecomConfigRequiresFields(t *testing.T) {
	if _, err := decodeWecomConfig([]byte(`{}`)); !errors.Is(err, ErrWecomNotConfigured) {
		t.Fatalf("direct config without fields error = %v, want ErrWecomNotConfigured", err)
	}
	if _, err := decodeWecomConfig([]byte(`{"mode":"center"}`)); !errors.Is(err, ErrWecomNotConfigured) {
		t.Fatalf("center config without fields error = %v, want ErrWecomNotConfigured", err)
	}
	if _, err := decodeWecomConfig([]byte(`{"mode":"other","corpId":"ww1","agentId":1,"secret":"s"}`)); err == nil || !strings.Contains(err.Error(), "unknown wecom mode") {
		t.Fatalf("unknown mode error = %v, want unknown wecom mode", err)
	}
}

func TestWecomDirectClientExchangeCachesToken(t *testing.T) {
	tokenRequests := 0
	userInfoRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			tokenRequests++
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token-1", "expires_in": 7200})
		case "/cgi-bin/auth/getuserinfo":
			userInfoRequests++
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "userid": "zhangsan"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &WecomDirectClient{cfg: WecomConfig{Mode: WecomModeDirect, CorpID: "ww1", AgentID: 1, Secret: "s"}, base: server.URL, httpc: server.Client(), now: time.Now}
	identity, err := client.Exchange(context.Background(), "code-1")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if identity.Userid != "zhangsan" || identity.Name != "" {
		t.Fatalf("identity = %+v, want userid zhangsan without name", identity)
	}
	if _, err = client.Exchange(context.Background(), "code-2"); err != nil {
		t.Fatalf("second Exchange returned error: %v", err)
	}
	if tokenRequests != 1 {
		t.Fatalf("gettoken requests = %d, want 1 (cached)", tokenRequests)
	}
	if userInfoRequests != 2 {
		t.Fatalf("getuserinfo requests = %d, want 2", userInfoRequests)
	}
}

func TestWecomDirectClientExchangeRejectsWecomError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token-1", "expires_in": 7200})
		case "/cgi-bin/auth/getuserinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40029, "errmsg": "invalid code"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &WecomDirectClient{cfg: WecomConfig{Mode: WecomModeDirect, CorpID: "ww1", AgentID: 1, Secret: "s"}, base: server.URL, httpc: server.Client(), now: time.Now}
	if _, err := client.Exchange(context.Background(), "bad-code"); err == nil || !strings.Contains(err.Error(), "40029") {
		t.Fatalf("Exchange error = %v, want errcode 40029", err)
	}
}

func TestWecomDirectClientAuthorizeURL(t *testing.T) {
	client := &WecomDirectClient{cfg: WecomConfig{Mode: WecomModeDirect, CorpID: "ww1", AgentID: 1000002}}
	qrURL := client.AuthorizeURL("https://dns.example.com/api/auth/wecom/callback", "state-1")
	if !strings.HasPrefix(qrURL, "https://login.work.weixin.qq.com/wwlogin/sso/login?") {
		t.Fatalf("qr authorize url = %q, want wwlogin entry", qrURL)
	}
	if !strings.Contains(qrURL, "login_type=CorpApp") || !strings.Contains(qrURL, "appid=ww1") || !strings.Contains(qrURL, "agentid=1000002") || !strings.Contains(qrURL, "state=state-1") {
		t.Fatalf("qr authorize url missing params: %q", qrURL)
	}
}

func TestWecomCenterClientExchangeSignsAndParses(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/verify" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		_ = json.NewEncoder(w).Encode(map[string]string{"userid": "zhangsan", "name": "张三"})
	}))
	defer server.Close()

	client := &WecomCenterClient{cfg: WecomConfig{Mode: WecomModeCenter, AuthCenterURL: server.URL, AppID: "zonelease", AppSecret: "shared-secret"}, httpc: server.Client()}
	identity, err := client.Exchange(context.Background(), "ticket-1")
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if identity.Userid != "zhangsan" || identity.Name != "张三" {
		t.Fatalf("identity = %+v, want zhangsan/张三", identity)
	}
	app, _ := received["app"].(string)
	ticket, _ := received["ticket"].(string)
	sign, _ := received["sign"].(string)
	tsValue, _ := received["ts"].(float64)
	if app != "zonelease" || ticket != "ticket-1" {
		t.Fatalf("verify payload = %+v, want app zonelease and ticket ticket-1", received)
	}
	if !VerifyWecomTicketSignature("shared-secret", app, ticket, sign, int64(tsValue)) {
		t.Fatalf("signature %q does not match expected algorithm", sign)
	}
}

func TestWecomCenterClientExchangePropagatesRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_ticket"})
	}))
	defer server.Close()

	client := &WecomCenterClient{cfg: WecomConfig{Mode: WecomModeCenter, AuthCenterURL: server.URL, AppID: "zonelease", AppSecret: "shared-secret"}, httpc: server.Client()}
	if _, err := client.Exchange(context.Background(), "bad-ticket"); err == nil || !strings.Contains(err.Error(), "invalid_ticket") {
		t.Fatalf("Exchange error = %v, want invalid_ticket rejection", err)
	}
}

func TestWecomCenterClientAuthorizeURL(t *testing.T) {
	client := &WecomCenterClient{cfg: WecomConfig{Mode: WecomModeCenter, AuthCenterURL: "https://auth.example.com", AppID: "zonelease"}}
	if got := client.AuthorizeURL("", ""); got != "https://auth.example.com/login?app=zonelease" {
		t.Fatalf("authorize url = %q, want auth center login entry", got)
	}
}

func TestWecomCenterClientPingChecksHealthz(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &WecomCenterClient{cfg: WecomConfig{Mode: WecomModeCenter, AuthCenterURL: server.URL, AppID: "zonelease", AppSecret: "s"}, httpc: server.Client()}
	if _, err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
}

func TestLoginTicketStoreOneTimeAndExpiry(t *testing.T) {
	store := newLoginTicketStore()
	now := time.Now()
	store.now = func() time.Time { return now }

	ticket, err := store.issue("session-token")
	if err != nil {
		t.Fatalf("issue returned error: %v", err)
	}
	token, ok := store.consume(ticket)
	if !ok || token != "session-token" {
		t.Fatalf("consume = (%q, %v), want session-token", token, ok)
	}
	if _, ok = store.consume(ticket); ok {
		t.Fatal("ticket should be one-time consumable")
	}

	expired, err := store.issue("expired-token")
	if err != nil {
		t.Fatalf("issue returned error: %v", err)
	}
	now = now.Add(wecomLoginTicketTTL + time.Second)
	if _, ok = store.consume(expired); ok {
		t.Fatal("expired ticket should not be consumable")
	}
}

func TestLoginByWecomRequiresBinding(t *testing.T) {
	store := &passwordResetStore{user: domainUserForTest("zhangsan"), wecomBound: true}
	service := New(store, Config{SessionSecret: "test-secret"})
	session, err := service.LoginByWecom(context.Background(), "wecom", WecomIdentity{Userid: "wecom-zhangsan", Name: "张三"})
	if err != nil {
		t.Fatalf("LoginByWecom returned error: %v", err)
	}
	if session.Provider != "wecom" || session.User.Username != "zhangsan" {
		t.Fatalf("session = %+v, want wecom provider and username zhangsan", session)
	}
}

func TestLoginByWecomRejectsUnboundAccount(t *testing.T) {
	store := &passwordResetStore{user: domainUserForTest("zhangsan")}
	service := New(store, Config{SessionSecret: "test-secret"})
	if _, err := service.LoginByWecom(context.Background(), "wecom", WecomIdentity{Userid: "nobody"}); !errors.Is(err, ErrWecomNotBound) {
		t.Fatalf("LoginByWecom error = %v, want ErrWecomNotBound", err)
	}
}

func TestLoginByWecomRejectsDisabledUser(t *testing.T) {
	user := domainUserForTest("zhangsan")
	user.Disabled = true
	store := &passwordResetStore{user: user, wecomBound: true}
	service := New(store, Config{SessionSecret: "test-secret"})
	if _, err := service.LoginByWecom(context.Background(), "wecom", WecomIdentity{Userid: "zhangsan"}); !errors.Is(err, ErrUserNotProvisioned) {
		t.Fatalf("LoginByWecom error = %v, want ErrUserNotProvisioned", err)
	}
}

func TestBindWecomConflictsWithOtherOwner(t *testing.T) {
	store := &passwordResetStore{user: domainUserForTest("lisi"), wecomBound: true}
	service := New(store, Config{SessionSecret: "test-secret"})
	if _, err := service.BindWecom(context.Background(), "user-admin", "wecom-lisi"); !errors.Is(err, ErrWecomConflict) {
		t.Fatalf("BindWecom error = %v, want ErrWecomConflict", err)
	}
	bound, err := service.BindWecom(context.Background(), "user-lisi", "wecom-lisi")
	if err != nil || bound != "wecom-lisi" {
		t.Fatalf("BindWecom = (%q, %v), want own binding to succeed", bound, err)
	}
}

func TestUnbindWecomReportsRemoval(t *testing.T) {
	store := &passwordResetStore{user: domainUserForTest("zhangsan"), wecomBound: true}
	service := New(store, Config{SessionSecret: "test-secret"})
	removed, err := service.UnbindWecom(context.Background(), "user-zhangsan")
	if err != nil || !removed {
		t.Fatalf("UnbindWecom = (%v, %v), want removed", removed, err)
	}
	removed, err = service.UnbindWecom(context.Background(), "user-zhangsan")
	if err != nil || removed {
		t.Fatalf("second UnbindWecom = (%v, %v), want idempotent false", removed, err)
	}
}

func TestSignWecomTicketMatchesDocumentedAlgorithm(t *testing.T) {
	sign := SignWecomTicket("secret", "zonelease", "ticket", 1700000000)
	if !VerifyWecomTicketSignature("secret", "zonelease", "ticket", sign, 1700000000) {
		t.Fatal("signature roundtrip failed")
	}
	if VerifyWecomTicketSignature("other", "zonelease", "ticket", sign, 1700000000) {
		t.Fatal("signature accepted wrong secret")
	}
	if VerifyWecomTicketSignature("secret", "zonelease", "ticket", sign, 1700000001) {
		t.Fatal("signature accepted wrong timestamp")
	}
}
