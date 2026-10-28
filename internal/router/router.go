// Package router wires the management API and the relay endpoints into a
// single http.Handler. It lives outside package main so both the server
// entry point and the end-to-end test suite (e2e/) build the exact same
// route table *and* the same middleware chain.
package router

import (
	"net/http"

	"starhub/controller"
	"starhub/middleware"
	"starhub/relay"
	"starhub/relay/dto"
	"starhub/web"
)

// New returns the full server handler: the embedded console wraps the
// management security and rate-limit middleware, which wrap the route
// table. The console is outermost so static assets and deep links are
// served without consuming rate-limit budget or tripping management
// security.
// Callers running under httptest add their own outer IPRateLimit; that is
// intentional and matches how the tests exercise the chain.
// Relay chain contract: TokenAuth (writes CtxUsingGroup) ->
// UserRateLimit -> TokenRateLimit -> Distribute (parses model/stream and
// caches the body) -> ModelRateLimit -> Relay (selects a channel and
// retries with failover).
func New() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/user/login", controller.Login)
	mux.HandleFunc("POST /api/user/register", controller.Register)
	mux.Handle("POST /api/user/logout", middleware.UserAuth(http.HandlerFunc(controller.Logout)))
	mux.Handle("GET /api/user/self", middleware.UserAuth(http.HandlerFunc(controller.GetSelf)))
	mux.Handle("PUT /api/user/self", middleware.UserAuth(http.HandlerFunc(controller.UpdateSelf)))
	mux.Handle("POST /api/user/deactivate", middleware.UserAuth(http.HandlerFunc(controller.DeactivateSelf)))
	mux.Handle("GET /api/user/sessions", middleware.UserAuth(http.HandlerFunc(controller.ListSessions)))
	mux.Handle("POST /api/user/sessions/terminate-others", middleware.UserAuth(http.HandlerFunc(controller.TerminateOtherSessions)))
	mux.Handle("GET /api/user/usage", middleware.UserAuth(http.HandlerFunc(controller.UserUsage)))
	mux.Handle("GET /api/user", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("GET /api/user/", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("PUT /api/user", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser))))
	mux.Handle("PUT /api/user/{id}/finance-role", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.SetUserFinanceRole))))
	mux.Handle("GET /api/finance/summary", middleware.FinanceAuth(http.HandlerFunc(controller.GetFinanceSummary)))
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

	mux.Handle("GET /api/resource", middleware.AdminAuth(http.HandlerFunc(controller.ListResources)))
	mux.Handle("GET /api/resource/publication-context", middleware.AdminAuth(http.HandlerFunc(controller.GetResourcePublicationContext)))
	mux.Handle("POST /api/resource/publication-preview", middleware.AdminAuth(http.HandlerFunc(controller.PreviewResourcePublication)))
	mux.Handle("PUT /api/resource", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateResources))))
	mux.Handle("GET /api/placement", middleware.AdminAuth(http.HandlerFunc(controller.ListResources)))
	mux.Handle("PUT /api/placement/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdatePlacement))))
	mux.Handle("GET /api/region", middleware.AdminAuth(http.HandlerFunc(controller.ListResources)))
	mux.Handle("PUT /api/region/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateResourceRegion))))
	mux.Handle("DELETE /api/region/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteResourceRegion))))

	mux.Handle("POST /api/token", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("POST /api/token/", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("GET /api/token", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("GET /api/token/", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("PUT /api/token", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("PUT /api/token/", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("DELETE /api/token/{id}", middleware.UserAuth(http.HandlerFunc(controller.DeleteToken)))
	mux.Handle("POST /api/token/{id}/regenerate", middleware.UserAuth(http.HandlerFunc(controller.RegenerateToken)))

	mux.Handle("GET /api/option", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("GET /api/option/", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("GET /api/option/status", middleware.AdminAuth(http.HandlerFunc(controller.OptionStatus)))
	mux.Handle("GET /api/option/pricing", middleware.AdminAuth(http.HandlerFunc(controller.GetPricingOptions)))
	mux.Handle("PUT /api/option/pricing", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdatePricingOptions))))
	mux.Handle("GET /api/option/time-pricing", middleware.AdminAuth(http.HandlerFunc(controller.GetTimePricing)))
	mux.Handle("PUT /api/option/time-pricing", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateTimePricing))))
	mux.Handle("POST /api/option/reload", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.ReloadOptions))))
	mux.Handle("GET /api/option/history", middleware.AdminAuth(http.HandlerFunc(controller.OptionHistory)))
	mux.Handle("POST /api/option/rollback", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.RollbackOption))))
	mux.Handle("PUT /api/option", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption))))
	mux.Handle("PUT /api/option/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption))))

	mux.Handle("GET /api/log", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))
	mux.Handle("GET /api/log/", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))

	mux.Handle("GET /api/audit", middleware.AdminAuth(http.HandlerFunc(controller.ListAuditLogs)))
	mux.Handle("GET /api/audit/", middleware.AdminAuth(http.HandlerFunc(controller.ListAuditLogs)))

	mux.Handle("GET /api/metrics", middleware.AdminAuth(http.HandlerFunc(controller.GetMetrics)))
	mux.Handle("GET /api/metrics/", middleware.AdminAuth(http.HandlerFunc(controller.GetMetrics)))

	// Model plaza: a read-only browse surface over the channel graph, the
	// metadata catalog and the live pricing options. These endpoints are
	// intentionally unauthenticated — they serve the public catalog, and a
	// visitor must be able to compare prices before signing up. When a valid
	// session cookie is present the handlers price the catalog through that
	// user's group, so a signed-in visitor sees their own numbers; an
	// anonymous visitor gets the default group's list price.
	//
	// None of these handlers touch the relay or the ledger.
	mux.HandleFunc("GET /api/plaza/models", controller.PlazaListModels)
	mux.HandleFunc("GET /api/plaza/models/{id...}", controller.PlazaGetModel)
	mux.HandleFunc("GET /api/plaza/filters", controller.PlazaFilters)
	mux.HandleFunc("GET /api/plaza/rank", controller.PlazaRank)

	// Metadata administration. Operators fill the catalog in; the plaza
	// degrades gracefully for any model that has none.
	mux.Handle("GET /api/plaza/meta", middleware.AdminAuth(http.HandlerFunc(controller.ListModelMeta)))
	mux.Handle("GET /api/plaza/meta/configuration", middleware.AdminAuth(http.HandlerFunc(controller.GetModelMetadataCatalog)))
	mux.Handle("PUT /api/plaza/meta", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpsertModelMeta))))
	mux.Handle("DELETE /api/plaza/meta/{model...}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteModelMeta))))

	mux.Handle("POST /api/redemption", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateRedemption))))
	mux.Handle("POST /api/redemption/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateRedemption))))
	mux.Handle("GET /api/redemption", middleware.AdminAuth(http.HandlerFunc(controller.ListRedemptions)))
	mux.Handle("GET /api/redemption/", middleware.AdminAuth(http.HandlerFunc(controller.ListRedemptions)))
	mux.Handle("DELETE /api/redemption/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteRedemption))))

	// User self-service: redeem a prepaid code.
	mux.Handle("POST /api/user/redeem", middleware.UserAuth(http.HandlerFunc(controller.RedeemCode)))

	// Wallet & billing: balance, ledger, recharge orders. All require an
	// authenticated user; admins see all records, users see their own.
	mux.Handle("GET /api/wallet", middleware.UserAuth(http.HandlerFunc(controller.GetWallet)))
	mux.Handle("GET /api/ledger", middleware.UserAuth(http.HandlerFunc(controller.ListLedger)))
	mux.Handle("GET /api/ledger/", middleware.UserAuth(http.HandlerFunc(controller.ListLedger)))
	mux.Handle("GET /api/recharge", middleware.UserAuth(http.HandlerFunc(controller.ListRechargeOrders)))
	mux.Handle("GET /api/recharge/", middleware.UserAuth(http.HandlerFunc(controller.ListRechargeOrders)))
	mux.Handle("POST /api/recharge", middleware.UserAuth(http.HandlerFunc(controller.CreateRechargeOrder)))
	mux.Handle("POST /api/recharge/", middleware.UserAuth(http.HandlerFunc(controller.CreateRechargeOrder)))
	mux.Handle("GET /api/recharge/{id}", middleware.UserAuth(http.HandlerFunc(controller.GetRechargeOrder)))
	mux.Handle("POST /api/recharge/{id}/begin", middleware.UserAuth(http.HandlerFunc(controller.BeginRechargePayment)))
	mux.Handle("POST /api/recharge/{id}/resolve", middleware.UserAuth(http.HandlerFunc(controller.ResolveRechargeOrder)))

	// Exports: server-generated CSV files from logs and ledger.
	mux.Handle("GET /api/export", middleware.UserAuth(http.HandlerFunc(controller.ListExports)))
	mux.Handle("GET /api/export/", middleware.UserAuth(http.HandlerFunc(controller.ListExports)))
	mux.Handle("POST /api/export", middleware.UserAuth(http.HandlerFunc(controller.CreateExport)))
	mux.Handle("POST /api/export/", middleware.UserAuth(http.HandlerFunc(controller.CreateExport)))
	mux.Handle("GET /api/export/{id}/download", middleware.UserAuth(http.HandlerFunc(controller.DownloadExport)))

	// Service group management (admin only). The multiplier is synced
	// to the group_ratio option so relay billing picks up changes.
	mux.Handle("GET /api/service-group", middleware.AdminAuth(http.HandlerFunc(controller.ListServiceGroups)))
	mux.Handle("GET /api/service-group/configuration", middleware.AdminAuth(http.HandlerFunc(controller.GetServiceGroupCatalog)))
	mux.Handle("GET /api/service-group/", middleware.AdminAuth(http.HandlerFunc(controller.ListServiceGroups)))
	mux.Handle("PUT /api/service-group", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpsertServiceGroup))))
	mux.Handle("PUT /api/service-group/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpsertServiceGroup))))
	mux.Handle("PUT /api/service-group/{id}/sla", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateServiceGroupSLA))))
	mux.Handle("DELETE /api/service-group/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteServiceGroup))))

	// Service quality: per-model SLA metrics aggregated from logs.
	mux.Handle("GET /api/service-quality", middleware.UserAuth(http.HandlerFunc(controller.GetServiceQuality)))
	mux.Handle("GET /api/service-quality/", middleware.UserAuth(http.HandlerFunc(controller.GetServiceQuality)))

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
	mux.Handle("POST /v1/responses", relayChain(dto.InboundOpenAI, controller.RelayResponses))
	mux.Handle("POST /v1beta/models/{method}", relayChain(dto.InboundGemini, controller.RelayGemini))
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
