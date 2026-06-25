package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"zonelease/backend/internal/domain"
)

var ErrNotificationChannelDisabled = errors.New("notification channel is disabled")

type ChannelStore interface {
	GetNotificationChannel(ctx context.Context, id string) (domain.NotificationChannel, error)
	GetPasswordResetNotificationChannel(ctx context.Context) (domain.NotificationChannel, error)
}

type Service struct {
	store ChannelStore
	http  *http.Client
	now   func() time.Time
}

type PasswordResetMessage struct {
	Username  string
	Code      string
	ExpiresAt time.Time
	RequestIP string
	To        string
}

type emailConfig struct {
	SMTPHost          string   `json:"smtpHost"`
	SMTPPort          int      `json:"smtpPort"`
	Username          string   `json:"username"`
	Password          string   `json:"password"`
	From              string   `json:"from"`
	FromName          string   `json:"fromName"`
	To                []string `json:"to"`
	UseTLS            bool     `json:"useTLS"`
	StartTLS          bool     `json:"startTLS"`
	AllowInsecureAuth bool     `json:"allowInsecureAuth"`
	templateConfig
}

func New(store ChannelStore) *Service {
	return &Service{store: store, http: &http.Client{Timeout: 8 * time.Second}, now: time.Now}
}

func (s *Service) SendPasswordResetCode(ctx context.Context, to, code string) error {
	return s.SendPasswordReset(ctx, to, code, s.now().Add(10*time.Minute))
}

func (s *Service) SendPasswordReset(ctx context.Context, to, code string, expiresAt time.Time) error {
	channel, err := s.store.GetPasswordResetNotificationChannel(ctx)
	if err != nil {
		return err
	}
	if !channel.PasswordResetEnabled {
		return ErrNotificationChannelDisabled
	}
	return s.SendPasswordResetMessage(ctx, channel, PasswordResetMessage{Code: code, ExpiresAt: expiresAt, To: to})
}

func (s *Service) SendPasswordResetMessage(ctx context.Context, channel domain.NotificationChannel, message PasswordResetMessage) error {
	switch channel.ID {
	case "email":
		var cfg emailConfig
		if err := decodeConfig(channel.Config, &cfg); err != nil {
			return err
		}
		return s.sendPasswordResetEmail(ctx, cfg, message)
	default:
		return fmt.Errorf("不支持的通知媒介 %s", channel.ID)
	}
}

func (s *Service) SendTest(ctx context.Context, channel domain.NotificationChannel, to string) error {
	if !channel.Enabled && !channel.PasswordResetEnabled {
		return ErrNotificationChannelDisabled
	}
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("收件人不能为空")
	}
	switch channel.ID {
	case "email":
		var cfg emailConfig
		if err := decodeConfig(channel.Config, &cfg); err != nil {
			return err
		}
		return s.sendTestEmail(ctx, cfg, to)
	default:
		return fmt.Errorf("不支持的通知媒介 %s", channel.ID)
	}
}

func (s *Service) send(ctx context.Context, channel domain.NotificationChannel, event notificationEvent) error {
	switch channel.ID {
	case "email":
		var cfg emailConfig
		if err := decodeConfig(channel.Config, &cfg); err != nil {
			return err
		}
		return s.sendEmail(ctx, cfg, event)
	default:
		return fmt.Errorf("不支持的通知媒介 %s", channel.ID)
	}
}

func UserFacingErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "通知发送已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "通知发送超时"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "通知发送超时"
	}
	message := err.Error()
	if strings.Contains(message, "不能为空") || strings.HasPrefix(message, "SMTP") || strings.HasPrefix(message, "通知") || strings.HasPrefix(message, "不支持的通知媒介") {
		return message
	}
	return "通知发送失败：" + message
}

func decodeConfig(data []byte, target any) error {
	if len(data) == 0 {
		return fmt.Errorf("通知配置为空")
	}
	return json.Unmarshal(data, target)
}
