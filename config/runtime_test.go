package config

import (
	"context"
	"testing"
	"time"
)

func TestRunLoopResumesAfterIntervalChange(t *testing.T) {
	previous, _ := SnapshotOption(OptionTaskPollInterval)
	t.Cleanup(func() { _ = ApplyOption(OptionTaskPollInterval, previous) })
	if err := ApplyOption(OptionTaskPollInterval, "0"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := make(chan struct{}, 3)
	go RunLoop(ctx, func(c Runtime) time.Duration { return c.TaskPollInterval },
		func(context.Context) { runs <- struct{}{} })
	select {
	case <-runs:
		t.Fatal("disabled loop ran")
	case <-time.After(30 * time.Millisecond):
	}
	if err := ApplyOption(OptionTaskPollInterval, "1"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runs:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("enabled loop did not run")
	}
	if err := ApplyOption(OptionTaskPollInterval, "0"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runs:
		t.Fatal("disabled loop ran again")
	case <-time.After(1100 * time.Millisecond):
	}
}

func TestNormalizeRuntimeOptionRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{OptionTaskPollInterval, "-1"},
		{OptionChannelTestInterval, "86401"},
		{OptionIPRateLimit, "0"},
		{OptionCookieSecure, "sometimes"},
	} {
		if _, err := NormalizeOption(tc.key, tc.value); err == nil {
			t.Fatalf("%s=%s should be rejected", tc.key, tc.value)
		}
	}
}
