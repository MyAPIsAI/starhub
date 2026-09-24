package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"starhub/common"
	"starhub/config"
)

const (
	DefaultIPLimitPerMinute   = 240
	DefaultUserLimitPerMinute = 120
	rateWindow                = time.Minute
	maxTrackedRateKeys        = 100000
)

type rateBucket struct {
	window int64
	count  int64
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	limit   int64
	buckets map[string]*rateBucket
}

func newFixedWindowLimiter(limit int64) *fixedWindowLimiter {
	return &fixedWindowLimiter{limit: limit, buckets: map[string]*rateBucket{}}
}

func (l *fixedWindowLimiter) allow(key string, now time.Time) bool {
	window := now.Unix() / int64(rateWindow/time.Second)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit == 0 {
		return true
	}
	b, ok := l.buckets[key]
	if !ok || b.window != window {
		if len(l.buckets) >= maxTrackedRateKeys {
			l.prune(window)
		}
		b = &rateBucket{window: window}
		l.buckets[key] = b
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

func (l *fixedWindowLimiter) prune(window int64) {
	for k, b := range l.buckets {
		if b.window != window {
			delete(l.buckets, k)
		}
	}
	if len(l.buckets) >= maxTrackedRateKeys {
		l.buckets = map[string]*rateBucket{}
	}
}

var (
	ipLimiter   = newFixedWindowLimiter(DefaultIPLimitPerMinute)
	userLimiter = newFixedWindowLimiter(DefaultUserLimitPerMinute)
)

func init() {
	config.Subscribe(func(c config.Runtime) {
		ConfigureRateLimits(c.IPLimitPerMinute, c.UserLimitPerMinute)
		ConfigureTokenRateLimit(c.TokenLimitPerMinute)
		ConfigureModelRateLimit(c.ModelLimitPerMinute)
	})
}

func ConfigureRateLimits(ipPerMinute, userPerMinute int64) {
	if ipPerMinute <= 0 {
		ipPerMinute = DefaultIPLimitPerMinute
	}
	if userPerMinute <= 0 {
		userPerMinute = DefaultUserLimitPerMinute
	}
	ipLimiter.mu.Lock()
	ipLimiter.limit = ipPerMinute
	ipLimiter.mu.Unlock()
	userLimiter.mu.Lock()
	userLimiter.limit = userPerMinute
	userLimiter.mu.Unlock()
}

func ResetRateLimits() {
	ipLimiter.mu.Lock()
	ipLimiter.buckets = map[string]*rateBucket{}
	ipLimiter.mu.Unlock()
	userLimiter.mu.Lock()
	userLimiter.buckets = map[string]*rateBucket{}
	userLimiter.mu.Unlock()
}

func IPRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ipLimiter.allow(clientIP(r), time.Now()) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(common.CtxUserID).(int64)
		if userID > 0 && !userLimiter.allow("user:"+strconv.FormatInt(userID, 10), time.Now()) {
			writeRateLimited(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeRateLimited(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", strconv.Itoa(int(rateWindow/time.Second)))
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		common.RelayError(w, http.StatusTooManyRequests,
			"rate_limit_error", "rate_limit_exceeded",
			"too many requests, please retry later")
		return
	}
	common.ApiError(w, http.StatusTooManyRequests, "too many requests, please retry later")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
