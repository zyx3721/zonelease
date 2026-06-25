package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func (s *Service) sendEmail(ctx context.Context, cfg emailConfig, event notificationEvent) error {
	if err := validateEmailRuntimeConfig(cfg, cfg.To...); err != nil {
		return err
	}
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	subject := alertSubject(event, cfg.templateConfig)
	message := []byte("From: " + formatMailFrom(cfg) + "\r\n" + "To: " + strings.Join(cfg.To, ",") + "\r\n" + "Subject: " + subject + "\r\n" + "Content-Type: " + emailContentType(cfg.templateConfig) + "; charset=UTF-8\r\n\r\n" + alertText(event, cfg.templateConfig))
	return s.sendSMTPMessage(ctx, cfg, addr, cfg.To, message)
}

func (s *Service) sendPasswordResetEmail(ctx context.Context, cfg emailConfig, message PasswordResetMessage) error {
	to := strings.TrimSpace(message.To)
	if err := validateEmailRuntimeConfig(cfg, to); err != nil {
		return err
	}
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	raw := []byte("From: " + formatMailFrom(cfg) + "\r\n" + "To: " + to + "\r\n" + "Subject: ZoneLease 密码找回验证码\r\n" + "MIME-Version: 1.0\r\n" + "Content-Type: text/html; charset=UTF-8\r\n\r\n" + passwordResetEmailHTML(message))
	return s.sendSMTPMessage(ctx, cfg, addr, []string{to}, raw)
}

func (s *Service) sendTestEmail(ctx context.Context, cfg emailConfig, to string) error {
	to = strings.TrimSpace(to)
	if err := validateEmailRuntimeConfig(cfg, to); err != nil {
		return err
	}
	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	raw := []byte("From: " + formatMailFrom(cfg) + "\r\n" + "To: " + to + "\r\n" + "Subject: ZoneLease 邮件配置测试\r\n" + "MIME-Version: 1.0\r\n" + "Content-Type: text/plain; charset=UTF-8\r\n\r\n" + "这是一封 ZoneLease 邮件配置测试邮件。")
	return s.sendSMTPMessage(ctx, cfg, addr, []string{to}, raw)
}

func validateEmailRuntimeConfig(cfg emailConfig, recipients ...string) error {
	if strings.TrimSpace(cfg.SMTPHost) == "" {
		return fmt.Errorf("SMTP 主机不能为空")
	}
	if cfg.SMTPPort <= 0 {
		return fmt.Errorf("SMTP 端口不能为空")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return fmt.Errorf("密码不能为空")
	}
	if strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("发件人不能为空")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return fmt.Errorf("发件人格式不正确")
	}
	if len(recipients) == 0 {
		return fmt.Errorf("收件人不能为空")
	}
	for _, recipient := range recipients {
		if _, err := mail.ParseAddress(recipient); err != nil {
			return fmt.Errorf("收件人格式不正确")
		}
	}
	return nil
}

func formatMailFrom(cfg emailConfig) string {
	name := strings.TrimSpace(cfg.FromName)
	if name == "" {
		return cfg.From
	}
	return (&mail.Address{Name: name, Address: cfg.From}).String()
}

func (s *Service) sendSMTPMessage(ctx context.Context, cfg emailConfig, addr string, recipients []string, message []byte) error {
	if cfg.UseTLS && cfg.StartTLS {
		return fmt.Errorf("TLS 与 STARTTLS 不能同时启用")
	}
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	var client *smtp.Client
	var err error
	if cfg.UseTLS {
		client, err = dialTLSSMTP(ctx, addr, cfg.SMTPHost, s.http.Timeout)
	} else if cfg.StartTLS {
		client, err = dialStartTLSSMTP(ctx, addr, cfg.SMTPHost, s.http.Timeout)
	} else {
		client, err = dialPlainSMTP(ctx, addr, cfg.SMTPHost, s.http.Timeout)
		if cfg.AllowInsecureAuth {
			auth = plainInsecureAuthPayload(cfg.Username, cfg.Password)
		}
	}
	if err != nil {
		return smtpError(err)
	}
	defer client.Quit()
	return smtpError(writeSMTPMessage(client, cfg.From, recipients, message, auth))
}

func writeSMTPMessage(client *smtp.Client, from string, recipients []string, message []byte, auth smtp.Auth) error {
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, to := range recipients {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

type plainInsecureAuth string

func plainInsecureAuthPayload(username string, password string) smtp.Auth {
	return plainInsecureAuth("\x00" + username + "\x00" + password)
}

func (a plainInsecureAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte(a), nil
}

func (a plainInsecureAuth) Next([]byte, bool) ([]byte, error) {
	return nil, nil
}

func dialPlainSMTP(ctx context.Context, addr string, host string, timeout time.Duration) (*smtp.Client, error) {
	conn, err := dialSMTPConn(ctx, addr, timeout)
	if err != nil {
		return nil, err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func dialTLSSMTP(ctx context.Context, addr string, host string, timeout time.Duration) (*smtp.Client, error) {
	conn, err := dialSMTPConn(ctx, addr, timeout)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
	if err := tlsConn.Handshake(); err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	client, err := smtp.NewClient(tlsConn, host)
	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	return client, nil
}

func dialStartTLSSMTP(ctx context.Context, addr string, host string, timeout time.Duration) (*smtp.Client, error) {
	client, err := dialPlainSMTP(ctx, addr, host, timeout)
	if err != nil {
		return nil, err
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		_ = client.Close()
		return nil, fmt.Errorf("SMTP 服务不支持 STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func dialSMTPConn(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	return conn, nil
}

func smtpError(err error) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "SMTP ") || strings.HasPrefix(err.Error(), "TLS ") {
		return err
	}
	return fmt.Errorf("SMTP 发送失败：%w", err)
}

func passwordResetEmailHTML(message PasswordResetMessage) string {
	username := strings.TrimSpace(message.Username)
	if username == "" {
		username = "当前账号"
	}
	requestIP := strings.TrimSpace(message.RequestIP)
	if requestIP == "" {
		requestIP = "未知"
	}
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;background:#f5f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#172033;">
<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#f5f7fb;padding:32px 12px;"><tr><td align="center">
<table role="presentation" width="560" cellspacing="0" cellpadding="0" style="max-width:560px;background:#ffffff;border:1px solid #e4e9f2;border-radius:14px;overflow:hidden;">
<tr><td style="padding:28px 32px 18px;background:#0f766e;color:#ffffff;"><div style="font-size:20px;font-weight:700;">ZoneLease 密码找回</div><div style="margin-top:8px;font-size:13px;opacity:.86;">请使用以下验证码完成密码重置</div></td></tr>
<tr><td style="padding:30px 32px;"><div style="font-size:14px;color:#526071;">账号</div><div style="margin-top:6px;font-size:18px;font-weight:700;color:#172033;">%s</div><div style="margin-top:24px;padding:18px 20px;border-radius:12px;background:#ecfdf5;border:1px solid #a7f3d0;text-align:center;"><div style="font-size:13px;color:#047857;">验证码</div><div style="margin-top:8px;font-size:34px;letter-spacing:8px;font-weight:800;color:#065f46;">%s</div></div><div style="margin-top:22px;font-size:14px;line-height:1.8;color:#526071;">有效期至：<strong style="color:#172033;">%s</strong><br>请求来源：<strong style="color:#172033;">%s</strong></div><div style="margin-top:24px;padding:14px 16px;border-radius:10px;background:#fff7ed;border:1px solid #fed7aa;color:#9a3412;font-size:13px;line-height:1.7;">如果不是您本人操作，请忽略本邮件并检查平台账号安全。</div></td></tr>
</table></td></tr></table></body></html>`, username, message.Code, message.ExpiresAt.Local().Format("2006-01-02 15:04:05"), requestIP)
}
