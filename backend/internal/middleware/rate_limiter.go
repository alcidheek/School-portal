package middleware

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/alcidheek/school-portal/internal/httpresponse"
)

type TokenBucket struct {
	Tokens     float64
	LastRefill time.Time
	LastSeen   time.Time
}

type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*TokenBucket
	capacity   float64
	refillRate float64
	now        func() time.Time
}

func NewRateLimiter(capacity float64, refillRate float64) *RateLimiter {
	// A request costs one token: smaller capacities could never admit a request.
	if capacity < 1 || capacity > 9007199254740991 || math.IsNaN(capacity) || math.IsInf(capacity, 0) {
		panic("rate limiter capacity must be finite, at least one token, and support exact one-token subtraction")
	}
	if refillRate <= 0 || math.IsNaN(refillRate) || math.IsInf(refillRate, 0) || math.IsInf(capacity/refillRate, 0) {
		panic("rate limiter refill rate must be positive and finite with a finite refill duration")
	}
	return &RateLimiter{
		buckets:    make(map[string]*TokenBucket),
		capacity:   capacity,
		refillRate: refillRate,
		now:        time.Now,
	}
}

func (rl *RateLimiter) Allow(clientID string) bool {
	allowed, _ := rl.allow(clientID)
	return allowed
}

// Admission and retry calculation use the same clock sample and mutex.
func (rl *RateLimiter) allow(clientID string) (bool, float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()

	bucket, exists := rl.buckets[clientID]

	if !exists {
		bucket = &TokenBucket{
			Tokens:     rl.capacity,
			LastRefill: now,
			LastSeen:   now,
		}
		rl.buckets[clientID] = bucket
	}

	elapsed := math.Max(0, now.Sub(bucket.LastRefill).Seconds())

	bucket.Tokens += elapsed * rl.refillRate

	if bucket.Tokens > rl.capacity {
		bucket.Tokens = rl.capacity
	}

	if now.After(bucket.LastRefill) {
		bucket.LastRefill = now
	}
	if now.After(bucket.LastSeen) {
		bucket.LastSeen = now
	}

	if bucket.Tokens < 1 {
		return false, math.Max(1, math.Ceil((1-bucket.Tokens)/rl.refillRate))
	}

	bucket.Tokens--
	return true, 0
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Identify the client using its IP address.
		clientID := getClientIP(r)

		// Check whether the client can make another request.
		if allowed, retrySeconds := rl.allow(clientID); !allowed {
			w.Header().Set("Retry-After", strconv.FormatFloat(retrySeconds, 'f', 0, 64))
			httpresponse.Error(
				w,
				"Too many requests. Please try again later.",
				http.StatusTooManyRequests,
			)
			return
		}

		// Allow the request to continue.
		next.ServeHTTP(w, r)
	})
}

func getClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Unmap().String()
	}
	// Real HTTP connections supply valid addresses. Malformed synthetic values
	// share one fallback bucket rather than creating attacker-chosen map keys.
	return "unknown-client"
}

// CleanupInactive preserves even rejected clients' recent activity. After a
// whole capacity/refillRate idle period, recreating the bucket grants no extra
// tokens compared with ordinary refill. Requests and cleanup share the mutex.
func (rl *RateLimiter) CleanupInactive() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	removed := 0
	for client, bucket := range rl.buckets {
		if now.Sub(bucket.LastSeen).Seconds() >= rl.capacity/rl.refillRate {
			delete(rl.buckets, client)
			removed++
		}
	}
	return removed
}

// The application owns this one synchronous loop and waits for it on shutdown.
// Constructing limiters or routers starts no background goroutines.
func RunRateLimiterCleanup(ctx context.Context, limiters ...*RateLimiter) {
	runRateLimiterCleanup(ctx, time.Minute, limiters...)
}

func runRateLimiterCleanup(ctx context.Context, interval time.Duration, limiters ...*RateLimiter) {
	if ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, limiter := range limiters {
				if ctx.Err() != nil {
					return
				}
				limiter.CleanupInactive()
			}
		}
	}
}
