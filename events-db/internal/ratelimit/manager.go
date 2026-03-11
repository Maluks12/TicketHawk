package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Manager struct {
	client   *redis.Client
	limiters map[string]*TokenBucketLimiter
}

func (m *Manager) Ping(ctx context.Context) error {
	return m.client.Ping(ctx).Err()
}

func (m *Manager) Client() *redis.Client {
	return m.client
}

func NewManager(redisAddr string) (*Manager, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		Password:     "",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis %w", err)
	}

	return &Manager{
		client:   client,
		limiters: make(map[string]*TokenBucketLimiter),
	}, nil
}

func (m *Manager) GetLimiter(name string, config BucketConfig) *TokenBucketLimiter {
	if limiter, exists := m.limiters[name]; exists {
		return limiter
	}

	limiter := NewTokenBucketLimiter(m.client, config)
	m.limiters[name] = limiter
	return limiter
}

func (m *Manager) Close() error {
	return m.client.Close()
}

func IPKey(category, ip string) string {
	return fmt.Sprintf("ratelimit:ip%s:%s", category, ip)
}

func UserKey(category, userID string) string {
	return fmt.Sprintf("ratelimit:user:%s:%s", category, userID)
}

func EndpointKey(endpoint, identifier string) string {
	return fmt.Sprintf("ratelimit:endpoint:%s:%s", endpoint, identifier)
}

func GlobalIPKey(ip string) string {
	return fmt.Sprintf("ratelimit:global:ip:%s", ip)
}

func GlobalUserKey(userID string) string {
	return fmt.Sprintf("ratelimit:global:user:%s", userID)
}