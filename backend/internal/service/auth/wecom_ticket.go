package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

const (
	wecomLoginTicketTTL  = 60 * time.Second
	wecomLoginTicketSize = 16 // 128 位
)

var (
	ErrWecomLoginTicket = errors.New("wecom login ticket is invalid or expired")
	ErrWecomNotBound    = errors.New("wecom account is not bound to a platform user")
	ErrWecomConflict    = errors.New("wecom account is already bound to another user")
)

// loginTicketStore 保存在回调阶段已建立、等待前端交换的一次性登录票据。
// 取出即删（防重放），进程重启即失效（票据只有 60 秒生命周期，重新登录即可）。
type loginTicketStore struct {
	mu      sync.Mutex
	tickets map[string]loginTicketRecord
	now     func() time.Time
}

type loginTicketRecord struct {
	token     string
	expiresAt time.Time
}

func newLoginTicketStore() *loginTicketStore {
	return &loginTicketStore{tickets: map[string]loginTicketRecord{}, now: time.Now}
}

func (s *loginTicketStore) issue(sessionToken string) (string, error) {
	buffer := make([]byte, wecomLoginTicketSize)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate login ticket: %w", err)
	}
	ticket := hex.EncodeToString(buffer)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.tickets[ticket] = loginTicketRecord{token: sessionToken, expiresAt: s.now().Add(wecomLoginTicketTTL)}
	return ticket, nil
}

func (s *loginTicketStore) consume(ticket string) (string, bool) {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.tickets[ticket]
	if !ok {
		return "", false
	}
	delete(s.tickets, ticket)
	if s.now().After(record.expiresAt) {
		return "", false
	}
	return record.token, true
}

func (s *loginTicketStore) pruneLocked() {
	now := s.now()
	for ticket, record := range s.tickets {
		if now.After(record.expiresAt) {
			delete(s.tickets, ticket)
		}
	}
}

// LoginByWecom 用企业微信身份建立平台会话：仅允许已绑定平台用户的企微账号登录。
func (s *Service) LoginByWecom(ctx context.Context, providerID string, identity WecomIdentity) (domain.Session, error) {
	identity.Userid = strings.TrimSpace(identity.Userid)
	if identity.Userid == "" {
		return domain.Session{}, ErrInvalidCredentials
	}
	user, err := s.store.FindUserByWecomBinding(ctx, identity.Userid)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.Session{}, ErrWecomNotBound
	}
	if err != nil {
		return domain.Session{}, err
	}
	if user.Disabled {
		return domain.Session{}, ErrUserNotProvisioned
	}
	return s.createSession(ctx, user, providerID)
}

// WecomBindingOf 返回用户绑定的企微 userid，未绑定时返回空串。
func (s *Service) WecomBindingOf(ctx context.Context, userID string) (string, bool, error) {
	userid, err := s.store.WecomBindingOfUser(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return userid, true, nil
}

// BindWecom 将企微 userid 绑定到用户；一个企微账号只能绑定一个用户，换绑覆盖自身旧绑定。
func (s *Service) BindWecom(ctx context.Context, userID, wecomUserid string) (string, error) {
	wecomUserid = strings.TrimSpace(wecomUserid)
	if wecomUserid == "" {
		return "", ErrInvalidCredentials
	}
	owner, err := s.store.FindWecomBindingOwner(ctx, wecomUserid)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return "", err
	}
	if err == nil && owner != userID {
		return "", ErrWecomConflict
	}
	if err := s.store.SaveWecomBinding(ctx, userID, wecomUserid); err != nil {
		return "", err
	}
	return wecomUserid, nil
}

// UnbindWecom 解除用户的企微绑定，未绑定时返回 false。
func (s *Service) UnbindWecom(ctx context.Context, userID string) (bool, error) {
	return s.store.DeleteWecomBinding(ctx, userID)
}

// IssueWecomLoginTicket 为已建立的会话签发一次性登录票据，供浏览器重定向后交换。
func (s *Service) IssueWecomLoginTicket(sessionToken string) (string, error) {
	return s.tickets.issue(sessionToken)
}

// ConsumeWecomLoginTicket 一次性消费登录票据，换取会话令牌。
func (s *Service) ConsumeWecomLoginTicket(ticket string) (string, bool) {
	return s.tickets.consume(ticket)
}
