package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"starhub/common"
	"starhub/config"
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
	startup, err := config.LoadStartup(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatalf("invalid startup configuration: %v", err)
	}
	if err := config.ApplyStartup(startup); err != nil {
		log.Fatalf("apply startup configuration: %v", err)
	}
	bootstrap := startup.Bootstrap
	if startup.ConfigPath != "" {
		log.Printf("configuration: %s", startup.ConfigPath)
	}
	if err := model.InitDB(bootstrap.DBPath); err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	rootPassword := bootstrap.RootPassword
	if rootPassword == "" {
		rootPassword = common.DefaultRootPasswd
		log.Print("WARNING: using default root password; set STARHUB_ROOT_PASSWORD to override")
	}
	if err := model.Seed(rootPassword); err != nil {
		log.Fatalf("failed to seed database: %v", err)
	}
	if err := model.LoadOptionsToSettings(); err != nil {
		log.Fatalf("failed to load options: %v", err)
	}
	go model.WatchOptions(context.Background(), time.Second)
	startChannelTestTask()
	startTaskPollTask()
	log.Printf("starhub %s listening on :%s (db: %s)", common.Version, bootstrap.Port, bootstrap.DBPath)
	// router.New() already carries the embedded console, management
	// security, and IP rate limiting around the route table.
	if err := http.ListenAndServe(":"+bootstrap.Port, router.New()); err != nil {
		log.Fatal(err)
	}
}

// startTaskPollTask starts the reloadable background task reaper.
func startTaskPollTask() {
	go config.RunLoop(context.Background(), func(c config.Runtime) time.Duration { return c.TaskPollInterval }, service.PollPendingTasksOnce)
	log.Printf("task poller configured (interval %s)", config.Current().TaskPollInterval)
}

// startChannelTestTask starts the reloadable channel health checker.
func startChannelTestTask() {
	go config.RunLoop(context.Background(), func(c config.Runtime) time.Duration { return c.ChannelTestInterval }, service.RunChannelTestOnce)
	log.Printf("channel health checker configured (interval %s)", config.Current().ChannelTestInterval)
}
