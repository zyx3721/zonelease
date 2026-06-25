package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

func (s *Service) CreateCaptcha(ctx context.Context) (domain.PasswordResetCaptcha, error) {
	left, err := randomInt(9)
	if err != nil {
		return domain.PasswordResetCaptcha{}, err
	}
	right, err := randomInt(9)
	if err != nil {
		return domain.PasswordResetCaptcha{}, err
	}
	answer := strconv.Itoa(left + 1 + right + 1)
	ttl := s.runtimeConfig(ctx).ResetCaptchaTTL
	expiresAt := s.now().Add(ttl)
	return domain.PasswordResetCaptcha{
		Token:     s.signCaptcha(answer, expiresAt),
		Question:  fmt.Sprintf("%d + %d = ?", left+1, right+1),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) VerifyResetIdentity(ctx context.Context, username, captchaToken, captchaAnswer string) (string, []domain.PasswordResetChannel, error) {
	if !s.verifyCaptcha(captchaToken, captchaAnswer) {
		return "", nil, ErrInvalidResetCaptcha
	}

	user, _, err := s.store.FindUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil || user.Disabled || user.Source != "local" || strings.TrimSpace(user.Email) == "" {
		return "", nil, ErrResetUnavailable
	}
	if s.notifier == nil {
		return "", nil, ErrResetChannelMissing
	}
	if _, err := s.store.GetPasswordResetNotificationChannel(ctx); err != nil {
		return "", nil, ErrResetChannelMissing
	}
	token, err := randomToken(32)
	if err != nil {
		return "", nil, err
	}
	if err := s.store.CreatePasswordResetRequest(ctx, token, user.ID, s.now().Add(s.cfg.ResetVerificationTTL)); err != nil {
		return "", nil, err
	}
	channels := []domain.PasswordResetChannel{{
		ID:         "email",
		Name:       "邮箱验证码",
		MaskedTo:   maskEmail(user.Email),
		RequiresTo: true,
	}}
	return token, channels, nil
}

func (s *Service) SendResetCode(ctx context.Context, verificationToken, channel, verifyEmail string) (int, string, error) {
	req, err := s.store.FindPasswordResetRequest(ctx, verificationToken)
	if err != nil || req.UsedAt != nil || !req.ExpiresAt.After(s.now()) {
		return 0, "", ErrInvalidResetToken
	}
	if strings.TrimSpace(req.UserEmail) == "" || channel != "email" || !strings.EqualFold(strings.TrimSpace(req.UserEmail), strings.TrimSpace(verifyEmail)) {
		return 0, "", ErrResetEmailMismatch
	}
	code, err := randomNumericCode(6)
	if err != nil {
		return 0, "", err
	}
	hash, err := repository.HashPassword(code)
	if err != nil {
		return 0, "", err
	}
	cfg := s.passwordResetRuntimeConfig(ctx)
	if err := s.ensureResetCodeAllowed(ctx, req.UserID, cfg); err != nil {
		return 0, "", err
	}
	if err := s.store.SetPasswordResetCode(ctx, verificationToken, hash, channel, s.now().Add(cfg.ResetCodeTTL)); err != nil {
		return 0, "", err
	}
	if s.notifier != nil {
		if err := s.notifier.SendPasswordReset(ctx, strings.TrimSpace(verifyEmail), code, s.now().Add(cfg.ResetCodeTTL)); err != nil {
			return 0, "", err
		}
	}
	return cfg.ResetSendCooldownSecs, code, nil
}

func (s *Service) ConfirmReset(ctx context.Context, username, verificationToken, code, newPassword string) error {
	req, err := s.store.FindPasswordResetRequest(ctx, verificationToken)
	if err != nil || req.UsedAt != nil || !req.ExpiresAt.After(s.now()) {
		return ErrInvalidResetToken
	}
	user, _, err := s.store.FindUserByUsername(ctx, username)
	if err != nil || user.ID != req.UserID || user.Disabled || user.Source != "local" || strings.TrimSpace(user.Email) == "" {
		return ErrResetUnavailable
	}
	if err := repository.VerifyPassword(req.CodeHash, code); err != nil {
		return ErrResetCodeMismatch
	}
	hash, err := repository.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.store.UpdateUserPassword(ctx, user.ID, hash); err != nil {
		return err
	}
	if err := s.store.MarkPasswordResetUsed(ctx, verificationToken); err != nil {
		return err
	}
	return s.store.DeleteUserSessions(ctx, user.ID)
}

func (s *Service) signCaptcha(answer string, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s:%d", answer, expiresAt.Unix())
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + signature))
}

func (s *Service) verifyCaptcha(token, answerText string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return false
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || !time.Unix(expiresUnix, 0).After(s.now()) {
		return false
	}
	payload := parts[0] + ":" + parts[1]
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	_, _ = mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return false
	}
	return strings.TrimSpace(answerText) == parts[0]
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomInt(max int) (int, error) {
	buf := make([]byte, 1)
	if _, err := rand.Read(buf); err != nil {
		return 0, err
	}
	return int(buf[0]) % max, nil
}

func randomNumericCode(size int) (string, error) {
	var b strings.Builder
	for b.Len() < size {
		value, err := randomInt(10)
		if err != nil {
			return "", err
		}
		b.WriteString(strconv.Itoa(value))
	}
	return b.String(), nil
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "未配置邮箱"
	}
	name := parts[0]
	if len(name) <= 2 {
		return name[:1] + "***@" + parts[1]
	}
	return name[:2] + "***@" + parts[1]
}
