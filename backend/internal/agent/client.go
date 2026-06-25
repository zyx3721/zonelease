package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

type Client struct {
	http *http.Client
}

type Envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *APIError       `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type HTTPStatusError struct {
	StatusCode int
	Status     string
	Code       string
	Message    string
}

func (e HTTPStatusError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("agent returned %s: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("agent returned %s", e.Status)
}

func NewClient() *Client {
	return &Client{http: &http.Client{}}
}

func (c *Client) withTLSInsecure(tlsInsecure bool) *Client {
	if !tlsInsecure {
		return c
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &Client{http: &http.Client{Timeout: c.http.Timeout, Transport: transport}}
}

func (c *Client) Health(ctx context.Context, endpoint, apiKey string, tlsInsecure ...bool) error {
	var payload map[string]any
	return c.withTLSInsecure(optionalBool(tlsInsecure)).do(ctx, http.MethodGet, endpoint, apiKey, "/health", nil, &payload)
}

func (c *Client) Validate(ctx context.Context, endpoint, apiKey, role string, tlsInsecure ...bool) error {
	client := c.withTLSInsecure(optionalBool(tlsInsecure))
	if err := client.Health(ctx, endpoint, apiKey); err != nil {
		return err
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return nil
	}
	switch role {
	case "dns":
		var payload []map[string]any
		return client.do(ctx, http.MethodGet, endpoint, apiKey, "/dns/zones", nil, &payload)
	case "dhcp":
		var payload map[string]any
		err := client.do(ctx, http.MethodGet, endpoint, apiKey, "/dhcp/probe", nil, &payload)
		if err == nil || isAgentNotFound(err) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unsupported agent role: %s", role)
	}
}

func isAgentNotFound(err error) bool {
	var statusErr HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound
}

func (c *Client) Get(ctx context.Context, endpoint, apiKey, path string, dst any, tlsInsecure ...bool) error {
	return c.withTLSInsecure(optionalBool(tlsInsecure)).do(ctx, http.MethodGet, endpoint, apiKey, path, nil, dst)
}

func (c *Client) Post(ctx context.Context, endpoint, apiKey, path string, body any, dst any, tlsInsecure ...bool) error {
	return c.withTLSInsecure(optionalBool(tlsInsecure)).do(ctx, http.MethodPost, endpoint, apiKey, path, body, dst)
}

func (c *Client) Delete(ctx context.Context, endpoint, apiKey, path string, dst any, tlsInsecure ...bool) error {
	return c.withTLSInsecure(optionalBool(tlsInsecure)).do(ctx, http.MethodDelete, endpoint, apiKey, path, nil, dst)
}

func (c *Client) Put(ctx context.Context, endpoint, apiKey, path string, body any, dst any, tlsInsecure ...bool) error {
	return c.withTLSInsecure(optionalBool(tlsInsecure)).do(ctx, http.MethodPut, endpoint, apiKey, path, body, dst)
}

func (c *Client) do(ctx context.Context, method, endpoint, apiKey, path string, body any, dst any) error {
	endpoint = strings.TrimRight(endpoint, "/")
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var env Envelope
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode agent response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || !env.Success {
		if env.Error != nil {
			return HTTPStatusError{StatusCode: res.StatusCode, Status: res.Status, Code: env.Error.Code, Message: env.Error.Message}
		}
		return HTTPStatusError{StatusCode: res.StatusCode, Status: res.Status}
	}
	if dst != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, dst); err != nil {
			return fmt.Errorf("decode agent data: %w", err)
		}
	}
	return nil
}

func UserFacingErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var statusErr HTTPStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized:
			return "Agent 认证失败，API Key 不正确或已失效"
		case http.StatusForbidden:
			return "Agent 拒绝访问，当前 API Key 没有权限"
		case http.StatusNotFound:
			return "Agent 接口不存在，请确认 Agent 地址是否填写到服务根路径，且角色选择正确"
		default:
			if strings.TrimSpace(statusErr.Message) != "" {
				return strings.TrimSpace(statusErr.Message)
			}
			return "Agent 返回异常状态：" + statusErr.Status
		}
	}
	if errors.Is(err, context.Canceled) {
		return "Agent 请求已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Agent 连接超时，请确认地址、端口和网络可达"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "Agent 连接超时，请确认地址、端口和网络可达"
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return "Agent TLS 证书不受信任，如使用自签名证书，可临时启用跳过 TLS 校验"
	}
	lowerMessage := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lowerMessage, "connection refused"):
		return "Agent 连接被拒绝，请确认 Agent 服务已启动，且端口开放"
	case strings.Contains(lowerMessage, "no such host"):
		return "Agent 地址无法解析，请检查域名或主机名是否正确"
	case strings.Contains(lowerMessage, "server gave http response to https client"):
		return "Agent 协议不匹配，当前使用 HTTPS 访问了 HTTP 服务"
	case strings.Contains(lowerMessage, "context deadline exceeded"):
		return "Agent 连接超时，请确认地址、端口和网络可达"
	case strings.Contains(lowerMessage, "unsupported agent role"):
		return "Agent 角色仅支持 DNS 或 DHCP"
	default:
		return "Agent 连接失败"
	}
}

func optionalBool(values []bool) bool {
	return len(values) > 0 && values[0]
}
