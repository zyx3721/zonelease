package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

func TestVerifyResetIdentityReportsUnavailableUserAfterValidCaptcha(t *testing.T) {
	ctx := context.Background()
	store := &passwordResetStore{findUserErr: repository.ErrNotFound}
	service := New(store, Config{SessionSecret: "test-secret", ResetCaptchaTTL: time.Minute, ResetVerificationTTL: 10 * time.Minute})

	captcha, err := service.CreateCaptcha(ctx)
	if err != nil {
		t.Fatalf("CreateCaptcha failed: %v", err)
	}

	_, _, err = service.VerifyResetIdentity(ctx, "missing", captcha.Token, answerForQuestion(t, captcha.Question))
	if !errors.Is(err, ErrResetUnavailable) {
		t.Fatalf("VerifyResetIdentity error = %v, want ErrResetUnavailable", err)
	}
}

func TestVerifyResetIdentityReportsInvalidCaptchaBeforeUserLookup(t *testing.T) {
	ctx := context.Background()
	store := &passwordResetStore{user: domain.User{ID: "user-1", Username: "alice", Email: "alice@example.com", Source: "local"}}
	service := New(store, Config{SessionSecret: "test-secret", ResetCaptchaTTL: time.Minute, ResetVerificationTTL: 10 * time.Minute})

	captcha, err := service.CreateCaptcha(ctx)
	if err != nil {
		t.Fatalf("CreateCaptcha failed: %v", err)
	}

	_, _, err = service.VerifyResetIdentity(ctx, "alice", captcha.Token, "wrong")
	if !errors.Is(err, ErrInvalidResetCaptcha) {
		t.Fatalf("VerifyResetIdentity error = %v, want ErrInvalidResetCaptcha", err)
	}
	if store.findUserCalled {
		t.Fatal("FindUserByUsername should not be called when captcha is invalid")
	}
}

func TestVerifyResetIdentityRequiresPasswordResetChannel(t *testing.T) {
	ctx := context.Background()
	store := &passwordResetStore{
		user:       domain.User{ID: "user-1", Username: "alice", Email: "alice@example.com", Source: "local"},
		channelErr: repository.ErrNotFound,
	}
	service := New(store, Config{SessionSecret: "test-secret", ResetCaptchaTTL: time.Minute, ResetVerificationTTL: 10 * time.Minute})
	service.SetNotifier(passwordResetNotifier{})

	captcha, err := service.CreateCaptcha(ctx)
	if err != nil {
		t.Fatalf("CreateCaptcha failed: %v", err)
	}

	_, _, err = service.VerifyResetIdentity(ctx, "alice", captcha.Token, answerForQuestion(t, captcha.Question))
	if !errors.Is(err, ErrResetChannelMissing) {
		t.Fatalf("VerifyResetIdentity error = %v, want ErrResetChannelMissing", err)
	}
}

func TestConfirmResetDeletesUserSessions(t *testing.T) {
	ctx := context.Background()
	code := "123456"
	hash, err := repository.HashPassword(code)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	store := &passwordResetStore{
		user: domain.User{ID: "user-1", Username: "alice", Email: "alice@example.com", Source: "local"},
		resetRequest: repository.PasswordResetRequest{
			UserID:    "user-1",
			UserEmail: "alice@example.com",
			CodeHash:  hash,
			Channel:   "email",
			ExpiresAt: time.Now().Add(time.Minute),
		},
	}
	service := New(store, Config{SessionSecret: "test-secret"})

	if err := service.ConfirmReset(ctx, "alice", "verify-token", code, "new-password"); err != nil {
		t.Fatalf("ConfirmReset failed: %v", err)
	}
	if !store.deleteUserSessionsCalled || store.deleteUserSessionsUserID != "user-1" {
		t.Fatalf("DeleteUserSessions called = %v userID = %q", store.deleteUserSessionsCalled, store.deleteUserSessionsUserID)
	}
}

func TestSendResetCodeReturnsCooldownRemainingSeconds(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	store := &passwordResetStore{
		resetRequest: repository.PasswordResetRequest{
			UserID:    "user-1",
			UserEmail: "alice@example.com",
			ExpiresAt: now.Add(time.Minute),
		},
		lastResetCodeSentAt: now.Add(-7 * time.Second),
		resetCodeCooling:    true,
	}
	service := New(store, Config{SessionSecret: "test-secret", ResetSendCooldownSecs: 30})
	service.now = func() time.Time { return now }
	service.SetNotifier(passwordResetNotifier{})

	_, _, err := service.SendResetCode(ctx, "verify-token", "email", "alice@example.com")
	var cooldownErr ResetCodeCooldownError
	if !errors.As(err, &cooldownErr) {
		t.Fatalf("SendResetCode error = %v, want ResetCodeCooldownError", err)
	}
	if cooldownErr.RemainingSeconds != 23 {
		t.Fatalf("RemainingSeconds = %d, want 23", cooldownErr.RemainingSeconds)
	}
}

type passwordResetStore struct {
	user                     domain.User
	resetRequest             repository.PasswordResetRequest
	lastResetCodeSentAt      time.Time
	resetCodeCooling         bool
	findUserErr              error
	channelErr               error
	findUserCalled           bool
	deleteUserSessionsCalled bool
	deleteUserSessionsUserID string
}

func (s *passwordResetStore) FindUserByUsername(context.Context, string) (domain.User, string, error) {
	s.findUserCalled = true
	return s.user, "", s.findUserErr
}

func (s *passwordResetStore) FindUserByID(context.Context, string) (domain.User, error) {
	return domain.User{}, repository.ErrNotFound
}

func (s *passwordResetStore) GetAuthProvider(context.Context, string) (domain.AuthProvider, error) {
	return domain.AuthProvider{}, repository.ErrNotFound
}

func (s *passwordResetStore) RecordUserLogin(context.Context, string) error {
	return nil
}

func (s *passwordResetStore) UpdateUserPassword(context.Context, string, string) error {
	return nil
}

func (s *passwordResetStore) CreateSession(context.Context, string, string, time.Time) error {
	return nil
}

func (s *passwordResetStore) FindSession(context.Context, string) (domain.User, time.Time, time.Time, error) {
	return domain.User{}, time.Time{}, time.Time{}, repository.ErrNotFound
}

func (s *passwordResetStore) TouchSession(context.Context, string) error {
	return nil
}

func (s *passwordResetStore) DeleteSession(context.Context, string) error {
	return nil
}

func (s *passwordResetStore) DeleteUserSessions(_ context.Context, userID string) error {
	s.deleteUserSessionsCalled = true
	s.deleteUserSessionsUserID = userID
	return nil
}

func (s *passwordResetStore) DeleteExpiredSessions(context.Context) error {
	return nil
}

func (s *passwordResetStore) CreatePasswordResetRequest(context.Context, string, string, time.Time) error {
	return nil
}

func (s *passwordResetStore) SetPasswordResetCode(context.Context, string, string, string, time.Time) error {
	return nil
}

func (s *passwordResetStore) FindPasswordResetRequest(context.Context, string) (repository.PasswordResetRequest, error) {
	if s.resetRequest.UserID == "" {
		return repository.PasswordResetRequest{}, repository.ErrNotFound
	}
	return s.resetRequest, nil
}

func (s *passwordResetStore) MarkPasswordResetUsed(context.Context, string) error {
	return nil
}

func (s *passwordResetStore) LatestRecentPasswordResetCodeSentAt(context.Context, string, time.Time) (time.Time, bool, error) {
	return s.lastResetCodeSentAt, s.resetCodeCooling, nil
}

func (s *passwordResetStore) CountRecentPasswordResetCodes(context.Context, string, time.Time) (int, error) {
	return 0, nil
}

func (s *passwordResetStore) GetSystemBaseConfig(context.Context) (domain.SystemBaseConfig, error) {
	return domain.DefaultSystemBaseConfig(), nil
}

func (s *passwordResetStore) GetPasswordResetNotificationChannel(context.Context) (domain.NotificationChannel, error) {
	if s.channelErr != nil {
		return domain.NotificationChannel{}, s.channelErr
	}
	return domain.NotificationChannel{ID: "email", PasswordResetEnabled: true}, nil
}

type passwordResetNotifier struct{}

func (passwordResetNotifier) SendPasswordResetCode(context.Context, string, string) error {
	return nil
}

func (passwordResetNotifier) SendPasswordReset(context.Context, string, string, time.Time) error {
	return nil
}

func answerForQuestion(t *testing.T, question string) string {
	t.Helper()
	var left, right int
	if _, err := fmt.Sscanf(strings.TrimSuffix(question, " = ?"), "%d + %d", &left, &right); err != nil {
		t.Fatalf("parse captcha question %q: %v", question, err)
	}
	return strconv.Itoa(left + right)
}
