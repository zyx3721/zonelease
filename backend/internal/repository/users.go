package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"zonelease/backend/internal/domain"
)

type UserUpsert struct {
	ID          string
	Username    string
	Email       string
	Password    string
	DisplayName string
	Role        string
	Disabled    bool
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, username, email, password_hash, display_name, role, COALESCE(NULLIF(source, ''), 'local'), disabled, last_login_at, created_at, updated_at
		FROM users ORDER BY created_at ASC, username ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		user, _, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachAccessToUsers(ctx, users)
}

func (s *Store) CreateUser(ctx context.Context, item UserUpsert) (domain.User, error) {
	passwordHash, err := HashPassword(item.Password)
	if err != nil {
		return domain.User{}, err
	}
	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users(username, email, password_hash, display_name, role, source, disabled)
		VALUES($1, $2, $3, $4, $5, 'local', $6)
		RETURNING id::text
	`, item.Username, item.Email, passwordHash, item.DisplayName, item.Role, item.Disabled).Scan(&id)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.replaceUserRoles(ctx, id, []string{item.Role}); err != nil {
		return domain.User{}, err
	}
	return s.FindUserByID(ctx, id)
}

func (s *Store) UpdateUser(ctx context.Context, item UserUpsert) (domain.User, error) {
	if strings.TrimSpace(item.Password) != "" {
		return s.updateUserWithPassword(ctx, item)
	}
	cmd, err := s.pool.Exec(ctx, `
		UPDATE users
		SET email=$2, display_name=$3, role=$4, disabled=$5, updated_at=now()
		WHERE id=$1
	`, item.ID, item.Email, item.DisplayName, item.Role, item.Disabled)
	if err != nil {
		return domain.User{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.User{}, ErrNotFound
	}
	if err := s.replaceUserRoles(ctx, item.ID, []string{item.Role}); err != nil {
		return domain.User{}, err
	}
	return s.FindUserByID(ctx, item.ID)
}

func (s *Store) updateUserWithPassword(ctx context.Context, item UserUpsert) (domain.User, error) {
	passwordHash, err := HashPassword(item.Password)
	if err != nil {
		return domain.User{}, err
	}
	cmd, err := s.pool.Exec(ctx, `
		UPDATE users
		SET email=$2, password_hash=$3, display_name=$4, role=$5, disabled=$6, updated_at=now()
		WHERE id=$1
	`, item.ID, item.Email, passwordHash, item.DisplayName, item.Role, item.Disabled)
	if err != nil {
		return domain.User{}, err
	}
	if cmd.RowsAffected() == 0 {
		return domain.User{}, ErrNotFound
	}
	if err := s.replaceUserRoles(ctx, item.ID, []string{item.Role}); err != nil {
		return domain.User{}, err
	}
	return s.FindUserByID(ctx, item.ID)
}

func scanUser(row pgx.Row) (domain.User, string, error) {
	var user domain.User
	var passwordHash string
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&passwordHash,
		&user.DisplayName,
		&user.Role,
		&user.Source,
		&user.Disabled,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, "", ErrNotFound
	}
	if user.Source == "" {
		user.Source = "local"
	}
	user.Permissions = PermissionsForRole(user.Role)
	return user, passwordHash, err
}

func FormatUserTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
