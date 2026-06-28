package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"zonelease/backend/internal/domain"
)

var ErrNotFound = errors.New("record not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyPassword(passwordHash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
}

func (s *Store) EnsureDefaultAdmin(ctx context.Context) error {
	var userCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}
	hash, err := HashPassword("123456")
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO users(username, password_hash, display_name, role)
		VALUES($1, $2, $3, 'admin')
	`, "admin", hash, "admin")
	return err
}

func PermissionsForRole(role string) []string {
	switch role {
	case "viewer":
		return []string{"dashboard.read", "dns.read", "dhcp.read", "servers.read", "audit.read", "notifications.read", "settings.base.read", "settings.users.read", "settings.auth.read", "settings.notifications.read"}
	case "operator":
		return []string{"dashboard.read", "dns.read", "dns.manage", "dhcp.read", "dhcp.manage", "servers.read", "servers.manage", "audit.read", "refresh.manage", "export.manage", "notifications.read", "notifications.manage", "settings.base.read", "settings.users.read", "settings.auth.read", "settings.notifications.read"}
	default:
		return []string{
			"dashboard.read", "dns.read", "dns.manage", "dhcp.read", "dhcp.manage",
			"servers.read", "servers.manage", "audit.read", "refresh.manage", "export.manage",
			"notifications.read", "notifications.manage",
			"settings.base.read", "settings.base.manage",
			"settings.users.read", "settings.users.manage",
			"settings.auth.read", "settings.auth.manage",
			"settings.notifications.read", "settings.notifications.manage",
		}
	}
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (domain.User, string, error) {
	user, passwordHash, err := scanUser(s.pool.QueryRow(ctx, `
		SELECT id::text, username, email, password_hash, display_name, role, COALESCE(NULLIF(source, ''), 'local'), disabled, last_login_at, created_at, updated_at
		FROM users WHERE username=$1
	`, username))
	if err != nil {
		return user, passwordHash, err
	}
	users, err := s.attachAccessToUsers(ctx, []domain.User{user})
	if err != nil {
		return user, passwordHash, err
	}
	return users[0], passwordHash, nil
}

func (s *Store) FindUserByID(ctx context.Context, id string) (domain.User, error) {
	user, _, err := scanUser(s.pool.QueryRow(ctx, `
		SELECT id::text, username, email, password_hash, display_name, role, COALESCE(NULLIF(source, ''), 'local'), disabled, last_login_at, created_at, updated_at
		FROM users WHERE id=$1
	`, id))
	if err != nil {
		return user, err
	}
	users, err := s.attachAccessToUsers(ctx, []domain.User{user})
	if err != nil {
		return user, err
	}
	return users[0], nil
}

func (s *Store) RecordUserLogin(ctx context.Context, userID string) error {
	cmd, err := s.pool.Exec(ctx, `UPDATE users SET last_login_at=now(), updated_at=now() WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	cmd, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func ClientIP(req *http.Request) string {
	for _, value := range []string{req.Header.Get("X-Forwarded-For"), req.Header.Get("X-Real-IP"), req.RemoteAddr} {
		if ip := normalizeClientIP(value); ip != "" {
			return ip
		}
	}
	return ""
}

func normalizeClientIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, ",") {
		value = strings.TrimSpace(strings.Split(value, ",")[0])
	}
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return host
	}
	return value
}
