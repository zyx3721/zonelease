package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

func (s *Store) FindUserByWecomBinding(ctx context.Context, wecomUserid string) (domain.User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.username, u.email, u.password_hash, u.display_name, u.role, COALESCE(NULLIF(u.source, ''), 'local'), u.disabled, u.last_login_at, u.created_at, u.updated_at
		FROM user_wecom_bindings w
		JOIN users u ON u.id = w.user_id
		WHERE w.wecom_userid = $1
	`, wecomUserid)
	user, _, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	users, err := s.attachAccessToUsers(ctx, []domain.User{user})
	if err != nil {
		return domain.User{}, err
	}
	return users[0], err
}

func (s *Store) FindWecomBindingOwner(ctx context.Context, wecomUserid string) (string, error) {
	var userID string
	err := s.pool.QueryRow(ctx, `SELECT user_id::text FROM user_wecom_bindings WHERE wecom_userid=$1`, wecomUserid).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (s *Store) SaveWecomBinding(ctx context.Context, userID, wecomUserid string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_wecom_bindings(user_id, wecom_userid) VALUES($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET wecom_userid = EXCLUDED.wecom_userid, bound_at = now()
	`, userID, wecomUserid)
	return err
}

func (s *Store) DeleteWecomBinding(ctx context.Context, userID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_wecom_bindings WHERE user_id=$1`, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) WecomBindingOfUser(ctx context.Context, userID string) (string, error) {
	var wecomUserid string
	err := s.pool.QueryRow(ctx, `SELECT wecom_userid FROM user_wecom_bindings WHERE user_id=$1`, userID).Scan(&wecomUserid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return wecomUserid, err
}

// wecomBoundFlags 批量返回用户的企业微信绑定状态，供用户信息组装统一填充。
func (s *Store) wecomBoundFlags(ctx context.Context, userIDs []string) (map[string]bool, error) {
	flags := make(map[string]bool, len(userIDs))
	if len(userIDs) == 0 {
		return flags, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT user_id::text FROM user_wecom_bindings WHERE user_id = ANY($1)
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		flags[userID] = true
	}
	return flags, rows.Err()
}
