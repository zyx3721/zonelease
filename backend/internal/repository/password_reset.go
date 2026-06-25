package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type PasswordResetRequest struct {
	TokenHash string
	UserID    string
	UserEmail string
	CodeHash  string
	Channel   string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (s *Store) CreatePasswordResetRequest(ctx context.Context, token, userID string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_reset_requests(token_hash, user_id, expires_at)
		VALUES($1, $2, $3)
	`, HashToken(token), userID, expiresAt)
	return err
}

func (s *Store) SetPasswordResetCode(ctx context.Context, token, codeHash, channel string, expiresAt time.Time) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE password_reset_requests
		SET code_hash=$2, channel=$3, expires_at=$4
		WHERE token_hash=$1 AND used_at IS NULL
	`, HashToken(token), codeHash, channel, expiresAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) LatestRecentPasswordResetCodeSentAt(ctx context.Context, userID string, since time.Time) (time.Time, bool, error) {
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT created_at FROM password_reset_requests
		WHERE user_id=$1 AND code_hash <> '' AND created_at >= $2
		ORDER BY created_at DESC
		LIMIT 1
	`, userID, since).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return createdAt, true, nil
}

func (s *Store) CountRecentPasswordResetCodes(ctx context.Context, userID string, since time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM password_reset_requests
		WHERE user_id=$1 AND code_hash <> '' AND created_at >= $2
	`, userID, since).Scan(&count)
	return count, err
}

func (s *Store) FindPasswordResetRequest(ctx context.Context, token string) (PasswordResetRequest, error) {
	var item PasswordResetRequest
	err := s.pool.QueryRow(ctx, `
		SELECT r.token_hash, r.user_id::text, u.email, r.code_hash, r.channel, r.expires_at, r.used_at
		FROM password_reset_requests r
		JOIN users u ON u.id = r.user_id
		WHERE token_hash=$1
	`, HashToken(token)).Scan(&item.TokenHash, &item.UserID, &item.UserEmail, &item.CodeHash, &item.Channel, &item.ExpiresAt, &item.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	return item, err
}

func (s *Store) MarkPasswordResetUsed(ctx context.Context, token string) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE password_reset_requests SET used_at=now()
		WHERE token_hash=$1 AND used_at IS NULL
	`, HashToken(token))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
