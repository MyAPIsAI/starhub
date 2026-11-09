// Command starhub-watch runs outside the gateway failure domain.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"starhub/observability"
	"time"
)

func main() {
	target := flag.String("target", "", "Authenticated gateway health URL")
	state := flag.String("state", "starhub-watch.json", "Durable notification outbox path")
	flag.Parse()
	c, err := observability.LoadConfig()
	if err != nil || *target == "" || c.ObserverToken == "" || observability.ValidateEndpoint(*target) != nil {
		observability.Log("", "watchdog_configuration", "invalid")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if err := observability.WatchOnce(ctx, c, *target, *state, time.Now()); err != nil {
			observability.Log("", "watchdog_cycle", "failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
