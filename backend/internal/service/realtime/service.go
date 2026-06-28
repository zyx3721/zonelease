package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	refreshChannel      = "zonelease:refresh-events"
	refreshStream       = "zonelease:refresh:stream"
	lastRefreshEventKey = "zonelease:refresh:last"

	defaultStreamMaxLen int64 = 10000
)

const unlockScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`

type Service struct {
	client       *redis.Client
	ttl          time.Duration
	streamMaxLen int64
}

type RefreshEvent struct {
	Type      string    `json:"type"`
	TaskID    string    `json:"taskId"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Lock struct {
	Key   string
	Token string
}

func New(client *redis.Client, ttl time.Duration) *Service {
	return NewWithStream(client, ttl, defaultStreamMaxLen)
}

func NewWithStream(client *redis.Client, ttl time.Duration, streamMaxLen int64) *Service {
	if streamMaxLen <= 0 {
		streamMaxLen = defaultStreamMaxLen
	}
	return &Service{client: client, ttl: ttl, streamMaxLen: streamMaxLen}
}

func Connect(ctx context.Context, addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}

func (s *Service) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	bytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, bytes, expiration).Err()
}

func (s *Service) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s *Service) Exists(ctx context.Context, key string) (bool, error) {
	count, err := s.client.Exists(ctx, key).Result()
	return count > 0, err
}

func (s *Service) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	raw, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) SetInt(ctx context.Context, key string, value int, expiration time.Duration) error {
	return s.client.Set(ctx, key, value, expiration).Err()
}

func (s *Service) GetInt(ctx context.Context, key string) (int, bool, error) {
	raw, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
}

func (s *Service) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func (s *Service) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *Service) TryLock(ctx context.Context, key string, expiration time.Duration) (Lock, bool, error) {
	if expiration <= 0 {
		expiration = time.Minute
	}
	token, err := randomToken()
	if err != nil {
		return Lock{}, false, err
	}
	ok, err := s.client.SetNX(ctx, key, token, expiration).Result()
	if err != nil || !ok {
		return Lock{}, ok, err
	}
	return Lock{Key: key, Token: token}, true, nil
}

func (s *Service) Unlock(ctx context.Context, lock Lock) error {
	if lock.Key == "" || lock.Token == "" {
		return nil
	}
	return s.client.Eval(ctx, unlockScript, []string{lock.Key}, lock.Token).Err()
}

func (s *Service) PublishRefresh(ctx context.Context, event RefreshEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	bytes, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := s.client.Set(ctx, lastRefreshEventKey, bytes, s.ttl).Err(); err != nil {
		return err
	}
	if err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: refreshStream,
		MaxLen: s.streamMaxLen,
		Approx: true,
		Values: map[string]any{"event": bytes},
	}).Err(); err != nil {
		return err
	}
	return s.client.Publish(ctx, refreshChannel, bytes).Err()
}

func (s *Service) SubscribeRefresh(ctx context.Context) *redis.PubSub {
	return s.client.Subscribe(ctx, refreshChannel)
}

func (s *Service) RecentRefreshEvents(ctx context.Context, limit int64) ([]string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	events, err := s.client.XRevRangeN(ctx, refreshStream, "+", "-", limit).Result()
	if err != nil {
		return nil, err
	}
	items := make([]string, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		raw := streamEventValue(events[i].Values["event"])
		if raw == "" {
			continue
		}
		items = append(items, raw)
	}
	return items, nil
}

func streamEventValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		if typed == nil {
			return ""
		}
		return fmt.Sprint(typed)
	}
}

func randomToken() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
