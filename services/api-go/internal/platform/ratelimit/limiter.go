package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type Memory struct {
	mu      sync.Mutex
	buckets map[string]bucket
}

type bucket struct {
	start time.Time
	count int
}

func NewMemory() *Memory {
	return &Memory{buckets: map[string]bucket{}}
}

func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	current := m.buckets[key]
	if current.start.IsZero() || now.Sub(current.start) >= window {
		current = bucket{start: now, count: 0}
	}
	current.count++
	m.buckets[key] = current
	return current.count <= limit, nil
}

type Redis struct {
	Client *redis.Client
}

func (r Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	count, err := r.Client.Incr(ctx, "lokasolve:rl:"+key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := r.Client.Expire(ctx, "lokasolve:rl:"+key, window).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(limit), nil
}
