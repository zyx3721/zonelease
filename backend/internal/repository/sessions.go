package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

func (s *Store) CreateSession(ctx context.Context, token string, userID string, provider string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions(token_hash, user_id, provider, expires_at, last_seen_at)
		VALUES($1, $2, COALESCE(NULLIF($3, ''), 'local'), $4, now())
	`, HashToken(token), userID, provider, expiresAt)
	return err
}

func (s *Store) FindSession(ctx context.Context, token string) (domain.User, string, time.Time, time.Time, error) {
	var user domain.User
	var provider string
	var expiresAt time.Time
	var lastSeenAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.username, u.email, u.display_name, u.role, COALESCE(NULLIF(u.source, ''), 'local'), u.disabled, u.last_login_at, u.created_at, u.updated_at, COALESCE(NULLIF(s.provider, ''), 'local'), s.expires_at, s.last_seen_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1
	`, HashToken(token)).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Source, &user.Disabled, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &provider, &expiresAt, &lastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, "", time.Time{}, time.Time{}, ErrNotFound
	}
	if err != nil {
		return domain.User{}, "", time.Time{}, time.Time{}, err
	}
	users, err := s.attachAccessToUsers(ctx, []domain.User{user})
	if err != nil {
		return domain.User{}, "", time.Time{}, time.Time{}, err
	}
	return users[0], provider, expiresAt, lastSeenAt, nil
}

func (s *Store) TouchSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET last_seen_at=now() WHERE token_hash=$1`, HashToken(token))
	return err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, HashToken(token))
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return err
}
