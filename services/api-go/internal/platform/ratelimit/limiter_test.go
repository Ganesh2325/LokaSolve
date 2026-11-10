package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLimiter(t *testing.T) {
	limiter := NewMemory()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ok, err := limiter.Allow(ctx, "login:test", 3, time.Minute)
		if err != nil || !ok {
			t.Fatalf("attempt %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := limiter.Allow(ctx, "login:test", 3, time.Minute)
	if err != nil || ok {
		t.Fatalf("expected denial, ok=%v err=%v", ok, err)
	}
}
