package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"starhub/common"
	"starhub/config"
	"starhub/controller"
	"starhub/internal/router"
	"starhub/metric"
	"starhub/middleware"
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
	cryptoSecretExplicit := os.Getenv("STARHUB_CRYPTO_SECRET") != ""
	if err := config.LoadLocalEnvironment(); err != nil {
		log.Fatalf("invalid local environment: %v", err)
	}
	startup, err := config.LoadStartup(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatalf("invalid startup configuration: %v", err)
	}
	// An explicit ring can be checked without initializing the legacy key.
	if os.Getenv("STARHUB_CREDENTIAL_KEYS") != "" || os.Getenv("STARHUB_CREDENTIAL_ACTIVE_KEY") != "" {
		if err := common.ValidateCredentialConfiguration(); err != nil {
			log.Fatal(err)
		}
	}
	generated, err := config.EnsurePersistentCryptoSecret(startup.Bootstrap.Database, true)
	if err != nil {
		log.Fatalf("crypto secret initialization failed: %v", err)
	}
	if generated {
		log.Print("crypto secret: generated and saved to .env; keep this file with your database backups")
	} else if cryptoSecretExplicit {
		log.Print("crypto secret: configured from explicit STARHUB_CRYPTO_SECRET")
	} else {
		log.Print("crypto secret: loaded from .env; ensure this file is backed up alongside the database")
	}
	if err := config.ValidateProduction(startup.Bootstrap, os.Getenv); err != nil {
		log.Fatal(err)
	}
	if err := common.ValidateCredentialConfiguration(); err != nil {
		log.Fatal(err)
	}
	if err := config.ApplyStartup(startup); err != nil {
		log.Fatalf("apply startup configuration: %v", err)
	}
	if os.Getenv("RESOURCE_CATALOG_NOTIFY_URL") != "" && (os.Getenv("NODE_NAME") == "" || len(os.Getenv("NODE_NAME")) > 128) {
		log.Fatal("resource platform integration requires a stable NODE_NAME of 1 to 128 bytes")
	}
	bootstrap := startup.Bootstrap
	if startup.ConfigPath != "" {
		log.Printf("configuration: %s", startup.ConfigPath)
	}
	if err := model.InitDBWithConfig(model.DatabaseConfig{
		Driver:          bootstrap.Database.Driver,
		DSN:             bootstrap.Database.DSN,
		MaxOpenConns:    bootstrap.Database.MaxOpenConns,
		MaxIdleConns:    bootstrap.Database.MaxIdleConns,
		ConnMaxLifetime: bootstrap.Database.ConnMaxLifetime,
	}); err != nil {
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
	if err := service.ConfigureMessageProviders(); err != nil {
		log.Fatal(err)
	}
	if _, err := service.DatabaseBackupInterval(); err != nil {
		log.Fatal(err)
	}
	log.Print("session storage: in-process memory; all sessions are lost on restart and not shared across instances")
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	run := func(task func(context.Context)) {
		workers.Add(1)
		go func() { defer workers.Done(); task(workerCtx) }()
	}
	run(func(ctx context.Context) { model.WatchOptions(ctx, time.Second) })
	middleware.SetTrustedProxies(os.Getenv("STARHUB_TRUSTED_PROXIES"))
	middleware.SetAdminIPWhitelist(os.Getenv("STARHUB_ADMIN_IP_WHITELIST"))
	middleware.ConfigureConcurrency(50, 0)
	run(startChannelTestTask)
	run(startTaskPollTask)
	run(func(ctx context.Context) {
		config.RunLoop(ctx, func(c config.Runtime) time.Duration {
			if c.TaskPollInterval <= 0 {
				return 30 * time.Second
			}
			return c.TaskPollInterval
		}, service.RetryPendingQuotaSettlementsOnce)
	})
	run(func(ctx context.Context) { service.RunRoutingChainCleanup(ctx, 15*time.Minute) })
	run(service.RunDailyReconciliationLoop)
	run(service.RunTokenPeriodRollover)
	metric.StartProduction(time.Now())
	run(service.RunProductionTelemetry)
	run(service.RunProductionAttemptDelivery)
	run(service.RunResourcePlatform)
	run(service.RunResourceObservations)
	run(service.RunResourceEventDelivery)
	run(service.RunLocalRoutingProbes)
	run(service.RunUpstreamProtection)
	run(service.RunSecurityNotifications)
	run(service.RunDatabaseBackups)
	address := net.JoinHostPort(bootstrap.BindAddress, bootstrap.Port)
	log.Printf("starhub %s listening on %s (db: %s)", common.Version, address, bootstrap.DBPath)
	// router.New() already carries the embedded console, management
	// security, and IP rate limiting around the route table.
	server := &http.Server{Addr: address, Handler: router.New(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	signals, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	serverFailed := false
	select {
	case <-signals.Done():
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			serverFailed = true
			log.Printf("server stopped: %v", err)
		}
	}
	controller.SetDraining(true)
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 30*time.Second)
	if err := server.Shutdown(drainCtx); err != nil {
		log.Printf("request drain timed out: %v", err)
		_ = server.Close()
	}
	cancelDrain()
	stopWorkers()
	stopped := make(chan struct{})
	go func() { workers.Wait(); close(stopped) }()
	select {
	case <-stopped:
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := service.StageProductionTelemetryContext(flushCtx); err != nil {
			log.Printf("final telemetry staging failed: %v", err)
		}
		if err := service.FlushProductionTelemetryContext(flushCtx, time.Now()); err != nil {
			log.Printf("final telemetry flush failed: %v", err)
		}
		cancel()
		if err := model.CloseDB(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		log.Print("background shutdown timed out; durable work will resume after restart")
	}
	if serverFailed {
		os.Exit(1)
	}
}

// startTaskPollTask starts the reloadable background task reaper.
func startTaskPollTask(ctx context.Context) {
	log.Printf("task poller configured (interval %s)", config.Current().TaskPollInterval)
	config.RunLoop(ctx, func(c config.Runtime) time.Duration { return c.TaskPollInterval }, service.PollPendingTasksOnce)
}

// startChannelTestTask starts the reloadable channel health checker.
func startChannelTestTask(ctx context.Context) {
	log.Printf("channel health checker configured (interval %s)", config.Current().ChannelTestInterval)
	config.RunLoop(ctx, func(c config.Runtime) time.Duration { return c.ChannelTestInterval }, service.RunChannelTestOnce)
}
