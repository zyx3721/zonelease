package router

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWecomStateRoundTrip(t *testing.T) {
	router := &Router{}
	router.cfg.Auth.SessionSecret = "test-secret"
	state, err := router.signWecomState()
	if err != nil {
		t.Fatalf("signWecomState returned error: %v", err)
	}
	if status := verifyWecomState(state, "test-secret"); status != wecomStateValid {
		t.Fatalf("verifyWecomState = %v, want valid", status)
	}
	if status := verifyWecomState(state, "other-secret"); status != wecomStateInvalid {
		t.Fatalf("verifyWecomState with wrong secret = %v, want invalid", status)
	}
	if status := verifyWecomState(state+".tampered", "test-secret"); status != wecomStateInvalid {
		t.Fatalf("verifyWecomState with tampered state = %v, want invalid", status)
	}
	if status := verifyWecomState("", "test-secret"); status != wecomStateInvalid {
		t.Fatalf("verifyWecomState with empty state = %v, want invalid", status)
	}
}

func TestWecomStateExpiry(t *testing.T) {
	router := &Router{}
	router.cfg.Auth.SessionSecret = "test-secret"
	state, err := router.signWecomState()
	if err != nil {
		t.Fatalf("signWecomState returned error: %v", err)
	}
	if status := verifyWecomStateAt(state, "test-secret", time.Now().Add(wecomStateTTL+time.Minute)); status != wecomStateExpired {
		t.Fatalf("verifyWecomStateAt after ttl = %v, want expired", status)
	}
	if status := verifyWecomStateAt(state, "test-secret", time.Now().Add(wecomStateTTL-time.Minute)); status != wecomStateValid {
		t.Fatalf("verifyWecomStateAt before ttl = %v, want valid", status)
	}
}

func TestWecomExternalBasePrefersConfiguredPrefix(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.local/api/auth/wecom/callback", nil)
	if got := wecomExternalBase(req, "https://dns.example.com/"); got != "https://dns.example.com" {
		t.Fatalf("wecomExternalBase with prefix = %q, want https://dns.example.com", got)
	}
	if got := wecomExternalBase(req, ""); got != "http://internal.local" {
		t.Fatalf("wecomExternalBase without prefix = %q, want http://internal.local", got)
	}
}

func TestWecomExternalBaseUsesProxyHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://internal.local/api/auth/wecom/callback", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "dns.example.com")
	if got := wecomExternalBase(req, ""); got != "https://dns.example.com" {
		t.Fatalf("wecomExternalBase with proxy headers = %q, want https://dns.example.com", got)
	}
}

func TestSanitizeWecomConfigDirectRequiresCredentials(t *testing.T) {
	config, err := sanitizeWecomConfigWithPrevious(
		map[string]any{"mode": "direct", "corpId": "ww1", "agentId": float64(1000002), "secret": ""},
		map[string]any{"secret": "saved-secret"},
		true,
	)
	if err != nil {
		t.Fatalf("sanitizeWecomConfigWithPrevious returned error: %v", err)
	}
	if stringValue(config["secret"]) != "saved-secret" {
		t.Fatalf("secret = %q, want saved-secret retained", stringValue(config["secret"]))
	}
	if _, ok := config["hasSecret"]; ok {
		t.Fatal("hasSecret marker should not be stored")
	}
	if agentID, ok := config["agentId"].(int); !ok || agentID != 1000002 {
		t.Fatalf("agentId = %#v, want int 1000002", config["agentId"])
	}

	if _, err = sanitizeWecomConfigWithPrevious(
		map[string]any{"mode": "direct", "corpId": "ww1", "agentId": float64(1)},
		map[string]any{},
		true,
	); err == nil || !strings.Contains(err.Error(), "应用 Secret 不能为空") {
		t.Fatalf("missing secret error = %v, want 应用 Secret 不能为空", err)
	}
}

func TestSanitizeWecomConfigCenterRequiresCredentials(t *testing.T) {
	config, err := sanitizeWecomConfigWithPrevious(
		map[string]any{"mode": "center", "authCenterUrl": "https://auth.example.com/", "appId": "zonelease", "appSecret": "s3cret"},
		map[string]any{},
		true,
	)
	if err != nil {
		t.Fatalf("sanitizeWecomConfigWithPrevious returned error: %v", err)
	}
	if stringValue(config["authCenterUrl"]) != "https://auth.example.com" {
		t.Fatalf("authCenterUrl = %q, want trailing slash removed", stringValue(config["authCenterUrl"]))
	}

	// 直连遗留字段在 center 模式下应被清理，废弃凭据不残留
	config, err = sanitizeWecomConfigWithPrevious(
		map[string]any{"mode": "center", "authCenterUrl": "https://auth.example.com", "appId": "zonelease", "appSecret": "s3cret", "corpId": "ww1", "secret": "legacy"},
		map[string]any{},
		true,
	)
	if err != nil {
		t.Fatalf("sanitizeWecomConfigWithPrevious returned error: %v", err)
	}
	if _, ok := config["secret"]; ok {
		t.Fatal("legacy direct secret should be removed in center mode")
	}

	if _, err = sanitizeWecomConfigWithPrevious(
		map[string]any{"mode": "center", "authCenterUrl": "https://auth.example.com", "appId": "zonelease"},
		map[string]any{},
		true,
	); err == nil || !strings.Contains(err.Error(), "应用对接密钥不能为空") {
		t.Fatalf("missing appSecret error = %v, want 应用对接密钥不能为空", err)
	}
}

func TestSanitizeWecomConfigDisabledKeepsPartialValues(t *testing.T) {
	config, err := sanitizeWecomConfigWithPrevious(
		map[string]any{"mode": "direct", "corpId": "ww1", "secret": "", "hasSecret": true},
		map[string]any{"secret": "saved-secret"},
		false,
	)
	if err != nil {
		t.Fatalf("sanitizeWecomConfigWithPrevious returned error: %v", err)
	}
	if _, ok := config["hasSecret"]; ok {
		t.Fatal("hasSecret marker should not be stored")
	}
	if value, ok := config["corpId"]; !ok || value != "ww1" {
		t.Fatalf("corpId = %#v, want ww1 kept when disabled", config["corpId"])
	}
}

func TestWecomErrorFromResolveMapping(t *testing.T) {
	cases := map[string]string{
		"wecom state expired":         wecomErrorExpiredState,
		"wecom state invalid":         wecomErrorInvalidState,
		"auth center verify http 502": wecomErrorCenterFailed,
		"getuserinfo errcode=40029":   wecomErrorWecomFailed,
	}
	for input, want := range cases {
		if got := wecomErrorFromResolve(errString(input)); got != want {
			t.Fatalf("wecomErrorFromResolve(%q) = %q, want %q", input, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
