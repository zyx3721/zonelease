package auth

import (
	"context"
	"math"
	"time"

	"zonelease/backend/internal/domain"
)

const (
	defaultResetSendCooldownSeconds = 30
	defaultResetRateLimitMax        = 5
)

func (s *Service) passwordResetRuntimeConfig(ctx context.Context) Config {
	cfg := s.cfg
	baseDefaults := domain.DefaultSystemBaseConfig()
	if cfg.ResetCodeTTL <= 0 {
		cfg.ResetCodeTTL = time.Duration(baseDefaults.ResetCodeTTLMinutes) * time.Minute
	}
	if cfg.ResetCaptchaTTL <= 0 {
		cfg.ResetCaptchaTTL = time.Duration(baseDefaults.ResetCaptchaTTLMinutes) * time.Minute
	}
	if cfg.ResetSendCooldownSecs <= 0 {
		cfg.ResetSendCooldownSecs = defaultResetSendCooldownSeconds
	}
	if cfg.ResetRateLimitSpan <= 0 {
		cfg.ResetRateLimitSpan = time.Duration(baseDefaults.PasswordResetRateLimitMinutes) * time.Minute
	}
	if cfg.ResetRateLimitMax <= 0 {
		cfg.ResetRateLimitMax = defaultResetRateLimitMax
	}

	base, err := s.store.GetSystemBaseConfig(ctx)
	if err != nil {
		return cfg
	}
	base = domain.NormalizeSystemBaseConfig(base)
	cfg.ResetCodeTTL = time.Duration(base.ResetCodeTTLMinutes) * time.Minute
	cfg.ResetCaptchaTTL = time.Duration(base.ResetCaptchaTTLMinutes) * time.Minute
	cfg.ResetSendCooldownSecs = int(base.PasswordResetSendCooldownMinutes * 60)
	cfg.ResetRateLimitSpan = time.Duration(base.PasswordResetRateLimitMinutes) * time.Minute
	return cfg
}

func (s *Service) runtimeConfig(ctx context.Context) Config {
	return s.passwordResetRuntimeConfig(ctx)
}

func (s *Service) ensureResetCodeAllowed(ctx context.Context, userID string, cfg Config) error {
	now := s.now()
	if cfg.ResetSendCooldownSecs > 0 {
		cooldownSince := now.Add(-time.Duration(cfg.ResetSendCooldownSecs) * time.Second)
		if lastSentAt, coolingDown, err := s.store.LatestRecentPasswordResetCodeSentAt(ctx, userID, cooldownSince); err != nil {
			return err
		} else if coolingDown {
			remaining := int(math.Ceil(lastSentAt.Add(time.Duration(cfg.ResetSendCooldownSecs) * time.Second).Sub(now).Seconds()))
			if remaining < 1 {
				remaining = 1
			}
			return ResetCodeCooldownError{RemainingSeconds: remaining}
		}
	}
	if cfg.ResetRateLimitSpan > 0 && cfg.ResetRateLimitMax > 0 {
		count, err := s.store.CountRecentPasswordResetCodes(ctx, userID, now.Add(-cfg.ResetRateLimitSpan))
		if err != nil {
			return err
		}
		if count >= cfg.ResetRateLimitMax {
			return ErrResetCodeRateLimited
		}
	}
	return nil
}
