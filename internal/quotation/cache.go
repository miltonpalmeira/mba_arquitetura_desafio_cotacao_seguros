package quotation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
)

// quoteCache keeps one partner quote per broker and risk.
// The value is the quote the partner returned, including partner and quote_id.
type quoteCache interface {
	Get(ctx context.Context, key string) (partner.Quote, bool)
	Set(ctx context.Context, key string, quote partner.Quote)
}

// cacheKey is the literal key from the SAD.
// tenant is the X-Tenant-Id header. The request body has no broker.
func cacheKey(tenant, partnerName string, request Request) string {
	return fmt.Sprintf("quote:%s:%s:%s:%d:%s:%s:%d:%d:%s",
		tenant,
		partnerName,
		digitsOnly(request.Driver.Document),
		request.Driver.BirthYear,
		plateKey(request.Vehicle.Plate),
		modelKey(request.Vehicle.Model),
		request.Vehicle.Year,
		request.Vehicle.ValueCents,
		request.Coverage,
	)
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func plateKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(value) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func modelKey(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(value), " "))
}

// RedisStore is the per-partner cache. A missing key, a bad value or a Redis
// error is a miss: the service buys the quote from the partner again.
// The TTL is absolute from the write. A hit does not extend it.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisStore(addr string, ttl time.Duration) (*RedisStore, error) {
	if ttl <= 0 {
		return nil, fmt.Errorf("cache ttl must be positive")
	}
	options, err := redisOptions(addr)
	if err != nil {
		return nil, err
	}
	return &RedisStore{client: redis.NewClient(options), ttl: ttl}, nil
}

func redisOptions(addr string) (*redis.Options, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("redis address is required")
	}
	if strings.Contains(addr, "://") {
		options, err := redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("redis address: %w", err)
		}
		return options, nil
	}
	return &redis.Options{Addr: addr}, nil
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) Get(ctx context.Context, key string) (partner.Quote, bool) {
	raw, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return partner.Quote{}, false
	}
	var quote partner.Quote
	if err := json.Unmarshal(raw, &quote); err != nil {
		return partner.Quote{}, false
	}
	return quote, true
}

func (s *RedisStore) Set(ctx context.Context, key string, quote partner.Quote) {
	raw, err := json.Marshal(quote)
	if err != nil {
		return
	}
	_ = s.client.Set(ctx, key, raw, s.ttl).Err()
}
