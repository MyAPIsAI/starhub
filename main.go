package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"starhub/common"
	"starhub/internal/router"
	"starhub/model"
	"starhub/service"
	"starhub/web"
)

func main() {
	if executable, err := os.Executable(); err == nil {
		log.Printf("executable: %s", executable)
	}
	if web.ConsoleEmbedded {
		log.Print("web console: embedded (open / in your browser)")
	} else {
		log.Print("web console: not embedded; this is an API-only build and / returns 404. Build with node scripts/build-release.mjs and run the binary from release/")
	}
	dbPath := os.Getenv("STARHUB_DB")
	if dbPath == "" {
		dbPath = "starhub.db"
	}
	port := os.Getenv("STARHUB_PORT")
	if port == "" {
		port = "3000"
	}
	if err := model.InitDB(dbPath); err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		log.Fatalf("failed to load options: %v", err)
	}
	rootPassword := os.Getenv("STARHUB_ROOT_PASSWORD")
	if rootPassword == "" {
		rootPassword = common.DefaultRootPasswd
		log.Printf("WARNING: using default root password %q, set STARHUB_ROOT_PASSWORD to override", rootPassword)
	}
	if err := model.Seed(rootPassword); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}
	startChannelTestTask()
	startTaskPollTask()
	log.Printf("starhub %s listening on :%s (db: %s)", common.Version, port, dbPath)
	// router.New() already carries the embedded console, management
	// security, and IP rate limiting around the route table.
	if err := http.ListenAndServe(":"+port, router.New()); err != nil {
		log.Fatal(err)
	}
}

// startTaskPollTask launches the background task reaper: clients that
// never poll their submissions still get finished and billed. The
// interval defaults to 60s and can be tuned or disabled (0) via
// STARHUB_TASK_POLL_INTERVAL (seconds).
func startTaskPollTask() {
	interval := 60 * time.Second
	if raw := os.Getenv("STARHUB_TASK_POLL_INTERVAL"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			log.Printf("WARNING: invalid STARHUB_TASK_POLL_INTERVAL %q, using default", raw)
		} else if v == 0 {
			interval = 0
		} else {
			interval = time.Duration(v) * time.Second
		}
	}
	if interval <= 0 {
		log.Printf("task poller disabled")
		return
	}
	go service.RunTaskPollLoop(context.Background(), interval)
	log.Printf("task poller started (interval %s)", interval)
}

// startChannelTestTask launches the background channel health checker. The
// interval defaults to service.DefaultChannelTestInterval and can be tuned or
// disabled (0) via STARHUB_CHANNEL_TEST_INTERVAL (seconds).
func startChannelTestTask() {
	interval := service.DefaultChannelTestInterval
	if raw := os.Getenv("STARHUB_CHANNEL_TEST_INTERVAL"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			log.Printf("WARNING: invalid STARHUB_CHANNEL_TEST_INTERVAL %q, using default", raw)
		} else if v == 0 {
			interval = 0
		} else {
			interval = time.Duration(v) * time.Second
		}
	}
	if interval <= 0 {
		log.Printf("channel health checker disabled")
		return
	}
	go service.RunChannelTestLoop(context.Background(), interval)
	log.Printf("channel health checker started (interval %s)", interval)
}
