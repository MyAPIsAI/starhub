package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"starhub/common"
	"starhub/controller"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/dto"
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
	if err := http.ListenAndServe(":"+port, NewRouter()); err != nil {
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

// NewRouter wires the management API and the relay endpoints.
// Relay chain contract: TokenAuth (writes CtxUsingGroup) ->
// UserRateLimit -> TokenRateLimit -> Distribute (parses model/stream and
// caches the body) -> ModelRateLimit -> Relay (selects a channel and
// retries with failover).
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/user/login", controller.Login)
	mux.HandleFunc("POST /api/user/register", controller.Register)
	mux.Handle("POST /api/user/logout", middleware.UserAuth(http.HandlerFunc(controller.Logout)))
	mux.Handle("GET /api/user/self", middleware.UserAuth(http.HandlerFunc(controller.GetSelf)))
	mux.Handle("GET /api/user/usage", middleware.UserAuth(http.HandlerFunc(controller.UserUsage)))
	mux.Handle("GET /api/user", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("GET /api/user/", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("PUT /api/user", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser))))
	mux.Handle("PUT /api/user/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser))))

	mux.Handle("POST /api/channel", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateChannel))))
	mux.Handle("POST /api/channel/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateChannel))))
	mux.Handle("POST /api/channel/test/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.TestChannel))))
	mux.Handle("GET /api/channel", middleware.AdminAuth(http.HandlerFunc(controller.ListChannels)))
	mux.Handle("GET /api/channel/", middleware.AdminAuth(http.HandlerFunc(controller.ListChannels)))
	mux.Handle("GET /api/channel/{id}", middleware.AdminAuth(http.HandlerFunc(controller.GetChannel)))
	mux.Handle("PUT /api/channel", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateChannel))))
	mux.Handle("PUT /api/channel/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateChannel))))
	mux.Handle("DELETE /api/channel/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteChannel))))

	mux.Handle("POST /api/token", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("POST /api/token/", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("GET /api/token", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("GET /api/token/", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("PUT /api/token", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("PUT /api/token/", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("DELETE /api/token/{id}", middleware.UserAuth(http.HandlerFunc(controller.DeleteToken)))

	mux.Handle("GET /api/option", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("GET /api/option/", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("PUT /api/option", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption))))
	mux.Handle("PUT /api/option/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption))))

	mux.Handle("GET /api/log", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))
	mux.Handle("GET /api/log/", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))

	mux.Handle("GET /api/audit", middleware.AdminAuth(http.HandlerFunc(controller.ListAuditLogs)))
	mux.Handle("GET /api/audit/", middleware.AdminAuth(http.HandlerFunc(controller.ListAuditLogs)))

	mux.Handle("GET /api/metrics", middleware.AdminAuth(http.HandlerFunc(controller.GetMetrics)))
	mux.Handle("GET /api/metrics/", middleware.AdminAuth(http.HandlerFunc(controller.GetMetrics)))

	mux.Handle("POST /api/redemption", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateRedemption))))
	mux.Handle("POST /api/redemption/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateRedemption))))
	mux.Handle("GET /api/redemption", middleware.AdminAuth(http.HandlerFunc(controller.ListRedemptions)))
	mux.Handle("GET /api/redemption/", middleware.AdminAuth(http.HandlerFunc(controller.ListRedemptions)))
	mux.Handle("DELETE /api/redemption/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteRedemption))))

	// User self-service: redeem a prepaid code.
	mux.Handle("POST /api/user/redeem", middleware.UserAuth(http.HandlerFunc(controller.RedeemCode)))

	// Relay endpoints (P3 multi-protocol family). All of them share the
	// TokenAuth -> UserRateLimit -> TokenRateLimit -> Distribute ->
	// ModelRateLimit -> Relay chain; the chat endpoints differ only in
	// the inbound protocol they parse.
	relayChain := func(inbound dto.InboundFormat, handler http.HandlerFunc) http.Handler {
		return middleware.TokenAuth(
			middleware.UserRateLimit(
				middleware.TokenRateLimit(
					middleware.Distribute(inbound)(
						middleware.ModelRateLimitWithOverrides(
							http.HandlerFunc(handler))))))
	}
	mux.Handle("POST /v1/chat/completions", relayChain(dto.InboundOpenAI, controller.RelayChat))
	mux.Handle("POST /v1/messages", relayChain(dto.InboundClaude, controller.RelayClaudeChat))
	mux.Handle("POST /v1/embeddings", relayChain(dto.InboundOpenAI, controller.RelayEmbeddings))
	mux.Handle("POST /v1/rerank", relayChain(dto.InboundOpenAI, controller.RelayRerank))
	mux.Handle("POST /v1/audio/speech", relayChain(dto.InboundOpenAI, controller.RelayTTS))
	mux.Handle("POST /v1/audio/transcriptions", relayChain(dto.InboundOpenAI, controller.RelaySTT))
	mux.Handle("POST /v1/audio/translations", relayChain(dto.InboundOpenAI, controller.RelaySTTTranslations))
	// Image generations tolerates a missing model: the distributor falls
	// back to the legacy default "dall-e" (P3.1); edits reads the model
	// from the multipart form.
	mux.Handle("POST /v1/images/generations", middleware.TokenAuth(
		middleware.UserRateLimit(
			middleware.TokenRateLimit(
				middleware.DistributeWithDefaultModel(dto.InboundOpenAI, "dall-e")(
					middleware.ModelRateLimitWithOverrides(
						http.HandlerFunc(controller.RelayImageGenerations)))))))
	mux.Handle("POST /v1/images/edits", relayChain(dto.InboundOpenAI, controller.RelayImageEdits))
	mux.Handle("POST /v1/video/generations", relayChain(dto.InboundOpenAI, controller.RelayTaskSubmitVideo))
	mux.Handle("GET /v1/video/generations/{id}", middleware.TokenAuth(http.HandlerFunc(controller.RelayTaskGetVideo)))

	// Realtime websocket: the model rides the query string, no body to
	// distribute.
	mux.Handle("GET /v1/realtime", middleware.TokenAuth(
		middleware.UserRateLimit(
			middleware.TokenRateLimit(http.HandlerFunc(relay.RelayRealtime)))))

	mux.Handle("GET /v1/models", middleware.TokenAuth(http.HandlerFunc(controller.ListModels)))

	return web.Handler(middleware.ManagementSecurity(middleware.IPRateLimit(mux)))
}
