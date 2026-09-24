package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Runtime struct {
	ChannelTestInterval time.Duration
	TaskPollInterval    time.Duration
	CookieSecure        bool
	IPLimitPerMinute    int64
	UserLimitPerMinute  int64
	TokenLimitPerMinute int64
	ModelLimitPerMinute int64
}

const (
	OptionChannelTestInterval = "channel_test_interval"
	OptionTaskPollInterval    = "task_poll_interval"
	OptionCookieSecure        = "cookie_secure"
	OptionIPRateLimit         = "ip_rate_limit"
	OptionUserRateLimit       = "user_rate_limit"
	OptionTokenRateLimit      = "token_rate_limit"
	OptionModelRateLimit      = "model_rate_limit"
)

var (
	mu        sync.Mutex
	current   = Runtime{ChannelTestInterval: 30 * time.Minute, TaskPollInterval: time.Minute, IPLimitPerMinute: 240, UserLimitPerMinute: 120, TokenLimitPerMinute: 60, ModelLimitPerMinute: 200}
	changed   = make(chan struct{})
	listeners []func(Runtime)
)

func OptionKeys() []string {
	return []string{OptionChannelTestInterval, OptionTaskPollInterval, OptionCookieSecure,
		OptionIPRateLimit, OptionUserRateLimit, OptionTokenRateLimit, OptionModelRateLimit}
}

func IsOptionKey(key string) bool {
	for _, known := range OptionKeys() {
		if key == known {
			return true
		}
	}
	return false
}

func Current() Runtime {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// Watch returns a consistent snapshot and a channel closed by the next update.
func Watch() (Runtime, <-chan struct{}) {
	mu.Lock()
	defer mu.Unlock()
	return current, changed
}

// RunLoop restarts its timer whenever the selected runtime interval changes.
func RunLoop(ctx context.Context, interval func(Runtime) time.Duration, work func(context.Context)) {
	for {
		snapshot, signal := Watch()
		period := interval(snapshot)
		if period <= 0 {
			select {
			case <-ctx.Done():
				return
			case <-signal:
				continue
			}
		}
		timer := time.NewTimer(period)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-signal:
			timer.Stop()
		case <-timer.C:
			work(ctx)
		}
	}
}

func Subscribe(fn func(Runtime)) {
	mu.Lock()
	listeners = append(listeners, fn)
	mu.Unlock()
	fn(Current())
}

func SnapshotOption(key string) (string, error) {
	c := Current()
	switch key {
	case OptionChannelTestInterval:
		return strconv.FormatInt(int64(c.ChannelTestInterval/time.Second), 10), nil
	case OptionTaskPollInterval:
		return strconv.FormatInt(int64(c.TaskPollInterval/time.Second), 10), nil
	case OptionCookieSecure:
		return strconv.FormatBool(c.CookieSecure), nil
	case OptionIPRateLimit:
		return strconv.FormatInt(c.IPLimitPerMinute, 10), nil
	case OptionUserRateLimit:
		return strconv.FormatInt(c.UserLimitPerMinute, 10), nil
	case OptionTokenRateLimit:
		return strconv.FormatInt(c.TokenLimitPerMinute, 10), nil
	case OptionModelRateLimit:
		return strconv.FormatInt(c.ModelLimitPerMinute, 10), nil
	default:
		return "", fmt.Errorf("unknown runtime option key: %s", key)
	}
}

// NormalizeOption validates a runtime option without changing live state.
func NormalizeOption(key, value string) (string, error) {
	value = strings.TrimSpace(value)
	if key == OptionCookieSecure {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return strconv.FormatBool(v), nil
	}
	if !IsOptionKey(key) {
		return "", fmt.Errorf("unknown runtime option key: %s", key)
	}
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil || v < 0 || v > 1_000_000 {
		return "", fmt.Errorf("invalid value for %s: expected integer in [0,1000000]", key)
	}
	if (key == OptionChannelTestInterval || key == OptionTaskPollInterval) && v > 86400 {
		return "", fmt.Errorf("invalid value for %s: interval exceeds one day", key)
	}
	if (key == OptionIPRateLimit || key == OptionUserRateLimit) && v == 0 {
		return "", fmt.Errorf("invalid value for %s: limit must be positive", key)
	}
	return strconv.FormatInt(v, 10), nil
}

func ApplyOption(key, value string) error {
	normalized, err := NormalizeOption(key, value)
	if err != nil {
		return err
	}
	mu.Lock()
	next := current
	switch key {
	case OptionChannelTestInterval:
		v, _ := strconv.ParseInt(normalized, 10, 64)
		next.ChannelTestInterval = time.Duration(v) * time.Second
	case OptionTaskPollInterval:
		v, _ := strconv.ParseInt(normalized, 10, 64)
		next.TaskPollInterval = time.Duration(v) * time.Second
	case OptionCookieSecure:
		next.CookieSecure = normalized == "true"
	case OptionIPRateLimit:
		next.IPLimitPerMinute, _ = strconv.ParseInt(normalized, 10, 64)
	case OptionUserRateLimit:
		next.UserLimitPerMinute, _ = strconv.ParseInt(normalized, 10, 64)
	case OptionTokenRateLimit:
		next.TokenLimitPerMinute, _ = strconv.ParseInt(normalized, 10, 64)
	case OptionModelRateLimit:
		next.ModelLimitPerMinute, _ = strconv.ParseInt(normalized, 10, 64)
	}
	if next == current {
		mu.Unlock()
		return nil
	}
	current = next
	close(changed)
	changed = make(chan struct{})
	callbacks := append([]func(Runtime){}, listeners...)
	mu.Unlock()
	for _, fn := range callbacks {
		fn(Current())
	}
	return nil
}
