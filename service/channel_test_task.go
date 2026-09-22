package service

import (
	"context"
	"log"
	"time"

	"starhub/common"
	"starhub/model"
)

// DefaultChannelTestInterval is the period of the background channel health
// checker when STARHUB_CHANNEL_TEST_INTERVAL is not set.
const DefaultChannelTestInterval = 30 * time.Minute

// RunChannelTestLoop periodically probes every channel until ctx is cancelled.
// main() starts it with a configurable interval; tests drive the checks
// manually through RunChannelTestOnce instead.
func RunChannelTestLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			RunChannelTestOnce(ctx)
		}
	}
}

// RunChannelTestOnce probes every channel that participates in automatic
// health management: enabled channels may get auto-disabled when the upstream
// proves broken (bad key, revoked access), auto-disabled channels get
// re-enabled once they answer again. Manually disabled channels are left
// alone; re-enabling those is the admin's call.
func RunChannelTestOnce(ctx context.Context) {
	channels, err := model.ListChannels()
	if err != nil {
		log.Printf("channel test task: list channels failed: %v", err)
		return
	}
	for i := range channels {
		ch := &channels[i]
		if ch.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		models := model.SplitList(ch.Models)
		if len(models) == 0 {
			continue
		}
		res := ProbeChannel(ctx, ch, models[0])
		status := ApplyProbeResult(ch, res)
		log.Printf("channel test: channel %d (%s) ok=%t status=%d: %s",
			ch.ID, ch.Name, res.OK, status, res.Message)
	}
	// After every probe cycle, scan the rolling error-rate windows and
	// fire threshold alerts for channels that crossed the limit.
	CheckErrorRateThresholds()
}
