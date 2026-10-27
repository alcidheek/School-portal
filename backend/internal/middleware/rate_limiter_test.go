package middleware

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterMiddleware(t *testing.T) {
	limiter, _ := clockedLimiter(5, 1.0/60.0*5)

	handler := limiter.Middleware(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))

	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(
			http.MethodPost,
			"/auth/login",
			nil,
		)

		// Same IP, different ports.
		req.RemoteAddr = "192.168.1.10:5000"

		if i%2 == 1 {
			req.RemoteAddr = "192.168.1.10:6000"
		}

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)

		expected := http.StatusOK
		if i == 5 {
			expected = http.StatusTooManyRequests
		}

		if recorder.Code != expected {
			t.Fatalf(
				"request %d: expected %d, got %d",
				i+1,
				expected,
				recorder.Code,
			)
		}
	}
}

func clockedLimiter(capacity, rate float64) (*RateLimiter, *atomic.Int64) {
	clock := &atomic.Int64{}
	limiter := NewRateLimiter(capacity, rate)
	limiter.now = func() time.Time { return time.Unix(1700000000, clock.Load()) }
	return limiter, clock
}

func TestRateLimiterRefillCapacityAndRetry(t *testing.T) {
	limiter, clock := clockedLimiter(5, 5.0/60)
	for i := 0; i < 5; i++ {
		if !limiter.Allow("client") {
			t.Fatal("initial burst denied")
		}
	}
	if allowed, retry := limiter.allow("client"); allowed || retry != 12 {
		t.Fatalf("retry %v %v", allowed, retry)
	}
	clock.Add(int64(6 * time.Second))
	if allowed, retry := limiter.allow("client"); allowed || retry != 6 {
		t.Fatalf("partial refill %v %v", allowed, retry)
	}
	clock.Add(int64(6 * time.Second))
	if !limiter.Allow("client") {
		t.Fatal("one token not replenished")
	}
	clock.Add(int64(time.Hour))
	if !limiter.Allow("client") || limiter.buckets["client"].Tokens != 4 {
		t.Fatal("refill exceeded capacity")
	}
	if !limiter.Allow("other-client") {
		t.Fatal("client buckets shared")
	}
	clock.Add(-int64(time.Hour))
	if !limiter.Allow("client") || limiter.buckets["client"].Tokens != 3 {
		t.Fatal("backward clock damaged balance")
	}
}

func TestRateLimiterRetryAfterJSON(t *testing.T) {
	limiter, clock := clockedLimiter(1, 0.4)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	request := httptest.NewRequest("POST", "/", nil)
	request.RemoteAddr = "192.0.2.1:1000"
	handler.ServeHTTP(httptest.NewRecorder(), request)
	for _, tc := range []struct {
		advance time.Duration
		retry   string
	}{{0, "3"}, {time.Second, "2"}, {1499 * time.Millisecond, "1"}} {
		clock.Add(int64(tc.advance))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 429 || w.Header().Get("Retry-After") != tc.retry || body["error"] != "Too many requests. Please try again later." || w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("retry response %d %s %s", w.Code, w.Header().Get("Retry-After"), w.Body)
		}
	}
}

func TestRateLimiterInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ capacity, rate float64 }{
		{0, 1}, {-1, 1}, {0.5, 1}, {math.NaN(), 1}, {math.Inf(1), 1}, {math.MaxFloat64, 1},
		{1, 0}, {1, -1}, {1, math.NaN()}, {1, math.Inf(1)}, {1, math.Inf(-1)}, {5, math.SmallestNonzeroFloat64},
	} {
		t.Run("invalid configuration", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid configuration accepted")
				}
			}()
			NewRateLimiter(tc.capacity, tc.rate)
		})
	}
}

func TestClientIPNormalizationAndUntrustedHeaders(t *testing.T) {
	for _, tc := range []struct{ remote, want string }{
		{"192.0.2.1:1000", "192.0.2.1"}, {"192.0.2.1:2000", "192.0.2.1"}, {"192.0.2.1", "192.0.2.1"},
		{"[2001:db8::1]:1000", "2001:db8::1"}, {"2001:0db8:0:0:0:0:0:1", "2001:db8::1"},
		{"[::ffff:192.0.2.1]:9000", "192.0.2.1"}, {"[fe80::1%eth0]:99", "fe80::1%eth0"}, {"malformed", "unknown-client"},
	} {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", "203.0.113.99")
		r.Header.Set("X-Real-IP", "203.0.113.98")
		if got := getClientIP(r); got != tc.want {
			t.Fatalf("%s: %s", tc.remote, got)
		}
	}
}

func TestRateLimiterConcurrentRequestsAndCleanup(t *testing.T) {
	limiter, _ := clockedLimiter(5, 5.0/60)
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter.CleanupInactive()
			if limiter.Allow("client") {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 5 {
		t.Fatalf("concurrent admission count %d", admitted.Load())
	}
}

func TestRateLimiterInactiveCleanupAndRecentDenials(t *testing.T) {
	limiter, clock := clockedLimiter(1, 1.0/60)
	limiter.Allow("inactive")
	limiter.Allow("active")
	clock.Add(int64(59 * time.Second))
	if limiter.Allow("active") {
		t.Fatal("early refill")
	}
	if limiter.CleanupInactive() != 0 {
		t.Fatal("premature cleanup")
	}
	clock.Add(int64(time.Second))
	if limiter.CleanupInactive() != 1 {
		t.Fatal("inactive bucket not removed")
	}
	if _, exists := limiter.buckets["active"]; !exists {
		t.Fatal("recently denied client removed")
	}
	if !limiter.Allow("inactive") {
		t.Fatal("fully refilled client denied")
	}
}

func TestRateLimiterCleanupCancellation(t *testing.T) {
	limiter, clock := clockedLimiter(1, 1.0/60)
	limiter.Allow("inactive")
	clock.Add(int64(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); runRateLimiterCleanup(ctx, time.Millisecond, limiter) }()
	deadline := time.After(time.Second)
	for {
		limiter.mu.Lock()
		empty := len(limiter.buckets) == 0
		limiter.mu.Unlock()
		if empty {
			break
		}
		select {
		case <-deadline:
			t.Fatal("periodic cleanup did not execute")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not stop")
	}
	limiter.Allow("after-stop")
	clock.Add(int64(time.Minute))
	time.Sleep(5 * time.Millisecond)
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.buckets) != 1 {
		t.Fatal("cleanup executed after stop")
	}
}
