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
	mux.Handle("GET /api/admin/reconciliation", middleware.AdminAuth(http.HandlerFunc(controller.AdminReconciliation)))
	mux.Handle("POST /api/admin/reconciliation", middleware.AdminAuth(http.HandlerFunc(controller.AdminReconciliation)))
	mux.Handle("GET /api/admin/customers", middleware.AdminAuth(http.HandlerFunc(controller.AdminCustomerList)))
	mux.Handle("GET /api/admin/customers/{id}", middleware.AdminAuth(http.HandlerFunc(controller.AdminCustomerDetail)))
	mux.Handle("GET /api/admin/customers/{id}/activity", middleware.AdminAuth(http.HandlerFunc(controller.AdminCustomerActivity)))
	mux.Handle("GET /api/admin/customers/{id}/calls/{trace}", middleware.AdminAuth(http.HandlerFunc(controller.AdminCustomerCall)))
	mux.Handle("POST /api/admin/customers/{id}/control", middleware.AdminAuth(http.HandlerFunc(controller.AdminCustomerControl)))
	mux.Handle("POST /api/admin/customers/{id}/sessions/revoke", middleware.AdminAuth(http.HandlerFunc(controller.AdminCustomerRevokeSessions)))
	mux.Handle("GET /api/admin/models", middleware.AdminAuth(http.HandlerFunc(controller.GetStandardModels)))
	mux.Handle("PUT /api/admin/models", middleware.AdminAuth(http.HandlerFunc(controller.PutStandardModel)))
	mux.Handle("DELETE /api/admin/models/{model...}", middleware.AdminAuth(http.HandlerFunc(controller.RemoveStandardModel)))
	mux.Handle("GET /api/admin/region-profile", middleware.AdminAuth(http.HandlerFunc(controller.GetDeploymentProfile)))
	mux.Handle("PUT /api/admin/region-profile", middleware.AdminAuth(http.HandlerFunc(controller.PutDeploymentProfile)))
	mux.Handle("GET /api/admin/legal", middleware.AdminAuth(http.HandlerFunc(controller.GetLegalVersions)))
	mux.Handle("POST /api/admin/legal", middleware.AdminAuth(http.HandlerFunc(controller.PostLegalVersion)))
	mux.HandleFunc("GET /api/legal/{kind}", controller.GetPublishedLegal)
	mux.Handle("GET /api/admin/pricing-history", middleware.AdminAuth(http.HandlerFunc(controller.ListPricingHistory)))
	staffPublic := func(handler http.HandlerFunc) http.Handler { return middleware.StaffNetwork(handler) }
	mux.Handle("GET /api/admin/install", staffPublic(controller.StaffInstallationState))
	mux.Handle("POST /api/admin/install", middleware.StaffNetwork(middleware.StaffLoginRateLimit(http.HandlerFunc(controller.InstallStaffRoot))))
	mux.Handle("POST /api/admin/auth/login", middleware.StaffNetwork(middleware.StaffLoginRateLimit(http.HandlerFunc(controller.StaffLogin))))
	mux.Handle("GET /api/admin/auth/state", staffPublic(controller.GetStaffAuthState))
	mux.Handle("POST /api/admin/auth/password", staffPublic(controller.StaffChangePassword))
	mux.Handle("POST /api/admin/auth/enroll", staffPublic(controller.StaffBeginEnrollment))
	mux.Handle("POST /api/admin/auth/verify", staffPublic(controller.StaffVerifyOTP))
	mux.Handle("POST /api/admin/auth/logout", staffPublic(controller.StaffLogout))
	mux.Handle("GET /api/admin/auth/self", middleware.StaffAuth(http.HandlerFunc(controller.GetStaffSelf)))
	mux.Handle("GET /api/admin/staff", middleware.AdminAuth(http.HandlerFunc(controller.ListStaffAccounts)))
	mux.Handle("POST /api/admin/staff", middleware.AdminAuth(http.HandlerFunc(controller.CreateStaffAccount)))
	mux.Handle("PUT /api/admin/staff/{id}", middleware.AdminAuth(http.HandlerFunc(controller.UpdateStaffAccount)))
	mux.Handle("POST /api/admin/customers/{id}/disable", middleware.AdminAuth(http.HandlerFunc(controller.StaffDisableCustomer)))
	mux.Handle("GET /api/admin/customers/{id}/keys", middleware.AdminAuth(http.HandlerFunc(controller.StaffCustomerKeys)))
	mux.Handle("POST /api/admin/customers/{id}/keys/{keyid}/disable", middleware.AdminAuth(http.HandlerFunc(controller.StaffDisableKey)))
	mux.Handle("POST /api/admin/customers/{id}/adjustments", middleware.AdminAuth(http.HandlerFunc(controller.StaffAdjustBalance)))
	mux.Handle("GET /api/admin/resource-sync", middleware.AdminAuth(http.HandlerFunc(controller.StaffResourceSyncStatus)))
	mux.Handle("POST /api/admin/resource-sync", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.StaffForceResourceSync))))
	mux.Handle("GET /api/admin/system", middleware.AdminAuth(http.HandlerFunc(controller.StaffSystemInfo)))

	mux.HandleFunc("POST /api/user/login", controller.Login)
	mux.HandleFunc("POST /api/user/register", controller.Register)
	mux.Handle("POST /api/user/logout", middleware.UserAuth(http.HandlerFunc(controller.Logout)))
	mux.Handle("POST /api/user/logout-current", middleware.UserAuth(http.HandlerFunc(controller.LogoutCurrent)))
	mux.Handle("GET /api/user/self", middleware.UserAuth(http.HandlerFunc(controller.GetSelf)))
	mux.Handle("PUT /api/user/self", middleware.UserAuth(http.HandlerFunc(controller.UpdateSelf)))
	mux.Handle("POST /api/user/deactivate", middleware.UserAuth(http.HandlerFunc(controller.DeactivateSelf)))
	mux.Handle("GET /api/user/sessions", middleware.UserAuth(http.HandlerFunc(controller.ListSessions)))
	mux.Handle("GET /api/user/session-state", middleware.UserAuth(http.HandlerFunc(controller.GetSessionState)))
	mux.Handle("GET /api/user/profile-state", middleware.UserAuth(http.HandlerFunc(controller.GetProfileState)))
	mux.Handle("GET /api/user/security-state", middleware.UserAuth(http.HandlerFunc(controller.GetUserSecurityState)))
	mux.Handle("GET /api/user/deactivation-state", middleware.UserAuth(http.HandlerFunc(controller.GetDeactivationState)))
	mux.HandleFunc("POST /api/user/deactivation-receipt", controller.GetDeactivationReceipt)
	mux.Handle("PUT /api/user/profile-state", middleware.UserAuth(middleware.PasswordVerificationRateLimit(http.HandlerFunc(controller.UpdateProfileState))))
	mux.Handle("POST /api/user/sessions/terminate-others", middleware.UserAuth(http.HandlerFunc(controller.TerminateOtherSessions)))
	mux.Handle("GET /api/user/usage", middleware.UserAuth(http.HandlerFunc(controller.UserUsage)))
	mux.Handle("GET /api/user/reports", middleware.UserAuth(http.HandlerFunc(controller.GetUsageReport)))
	mux.Handle("GET /api/user", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("GET /api/user/", middleware.AdminAuth(http.HandlerFunc(controller.ListUsers)))
	mux.Handle("PUT /api/user", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser))))
	mux.Handle("PUT /api/user/{id}/finance-role", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.SetUserFinanceRole))))
	mux.Handle("GET /api/finance/summary", middleware.FinanceAuth(http.HandlerFunc(controller.GetFinanceSummary)))
	mux.Handle("PUT /api/user/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateUser))))

	mux.Handle("GET /api/task-plugin", middleware.AdminAuth(http.HandlerFunc(controller.ListTaskPlugins)))
	mux.Handle("POST /api/task-plugin", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UploadTaskPlugin))))
	mux.Handle("PUT /api/task-plugin/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.SetTaskPluginEnabled))))
	mux.Handle("POST /api/task-plugin/{id}/test", middleware.AdminAuth(http.HandlerFunc(controller.TestTaskPlugin)))

	mux.Handle("POST /api/channel", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateChannel))))
	mux.Handle("POST /api/channel/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.CreateChannel))))
	mux.Handle("POST /api/channel/test/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.TestChannel))))
	mux.Handle("GET /api/channel", middleware.AdminAuth(http.HandlerFunc(controller.ListChannels)))
	mux.Handle("GET /api/channel/", middleware.AdminAuth(http.HandlerFunc(controller.ListChannels)))
	mux.Handle("GET /api/channel/{id}", middleware.AdminAuth(http.HandlerFunc(controller.GetChannel)))
	mux.Handle("PUT /api/channel", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateChannel))))
	mux.Handle("PUT /api/channel/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateChannel))))
	mux.Handle("DELETE /api/channel/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteChannel))))
	mux.Handle("GET /api/channel/{id}/upstream-usage", middleware.AdminAuth(http.HandlerFunc(controller.GetChannelUpstreamUsage)))

	mux.Handle("GET /api/resource", middleware.AdminAuth(http.HandlerFunc(controller.ListResources)))
	mux.Handle("GET /api/resource/publication-context", middleware.AdminAuth(http.HandlerFunc(controller.GetResourcePublicationContext)))
	mux.Handle("GET /api/resource/publication-state", middleware.AdminAuth(http.HandlerFunc(controller.GetResourceBindingState)))
	mux.Handle("PUT /api/resource/bindings/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.ChangeResourceBinding))))
	mux.Handle("GET /api/resource/binding-changes/{request_id}", middleware.AdminAuth(http.HandlerFunc(controller.GetResourceBindingChange)))
	mux.Handle("POST /api/resource/publication-preview", middleware.AdminAuth(http.HandlerFunc(controller.PreviewResourcePublication)))
	mux.Handle("PUT /api/resource/publication", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.PublishResourceGateway))))
	mux.Handle("GET /api/resource/publication-receipt/{digest}", middleware.AdminAuth(http.HandlerFunc(controller.GetResourcePublicationReceipt)))
	mux.Handle("PUT /api/resource", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateResources))))
	mux.Handle("GET /api/placement", middleware.AdminAuth(http.HandlerFunc(controller.ListResources)))
	mux.Handle("PUT /api/placement/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdatePlacement))))
	mux.Handle("GET /api/region", middleware.AdminAuth(http.HandlerFunc(controller.ListResources)))
	mux.Handle("PUT /api/region/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateResourceRegion))))
	mux.Handle("DELETE /api/region/{id}", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.DeleteResourceRegion))))

	mux.Handle("POST /api/token", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("GET /api/token/owned", middleware.UserAuth(http.HandlerFunc(controller.ListOwnedTokens)))
	mux.Handle("GET /api/token/{id}", middleware.UserAuth(http.HandlerFunc(controller.GetOwnedToken)))
	mux.Handle("GET /api/token/{id}/usage", middleware.UserAuth(http.HandlerFunc(controller.GetOwnedTokenUsage)))
	mux.Handle("GET /api/token/owned-choices", middleware.UserAuth(http.HandlerFunc(controller.GetOwnedTokenChoices)))
	mux.Handle("POST /api/token/owned-action", middleware.UserAuth(http.HandlerFunc(controller.WriteOwnedToken)))
	mux.Handle("POST /api/token/owned-receipt", middleware.UserAuth(http.HandlerFunc(controller.GetOwnedTokenReceipt)))
	mux.Handle("POST /api/token/", middleware.UserAuth(http.HandlerFunc(controller.CreateToken)))
	mux.Handle("GET /api/token", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("GET /api/token/", middleware.UserAuth(http.HandlerFunc(controller.ListTokens)))
	mux.Handle("PUT /api/token", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("PUT /api/token/", middleware.UserAuth(http.HandlerFunc(controller.UpdateToken)))
	mux.Handle("DELETE /api/token/{id}", middleware.UserAuth(http.HandlerFunc(controller.DeleteToken)))
	mux.Handle("POST /api/token/{id}/regenerate", middleware.UserAuth(http.HandlerFunc(controller.RegenerateToken)))
	mux.Handle("POST /api/token/{id}/verify-password", middleware.UserAuth(middleware.PasswordVerificationRateLimit(http.HandlerFunc(controller.VerifyTokenPassword))))
	mux.Handle("POST /api/token/{id}/reveal", middleware.UserAuth(http.HandlerFunc(controller.RevealToken)))

	mux.Handle("GET /api/option", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("GET /api/option/", middleware.AdminAuth(http.HandlerFunc(controller.ListOptions)))
	mux.Handle("GET /api/option/status", middleware.AdminAuth(http.HandlerFunc(controller.OptionStatus)))
	mux.Handle("GET /api/option/pricing", middleware.AdminAuth(http.HandlerFunc(controller.GetPricingOptions)))
	mux.Handle("PUT /api/option/pricing", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdatePricingOptions))))
	mux.Handle("GET /api/option/time-pricing", middleware.AdminAuth(http.HandlerFunc(controller.GetTimePricing)))
	mux.Handle("GET /api/option/time-token-pricing", middleware.AdminAuth(http.HandlerFunc(controller.GetTokenTimePricing)))
	mux.Handle("PUT /api/option/time-token-pricing", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateTokenTimePricing))))
	mux.Handle("PUT /api/option/time-pricing", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateTimePricing))))
	mux.Handle("POST /api/option/reload", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.ReloadOptions))))
	mux.Handle("GET /api/option/history", middleware.AdminAuth(http.HandlerFunc(controller.OptionHistory)))
	mux.Handle("POST /api/option/rollback", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.RollbackOption))))
	mux.Handle("PUT /api/option", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption))))
	mux.Handle("PUT /api/option/", middleware.Audit(middleware.AdminAuth(http.HandlerFunc(controller.UpdateOption))))

	mux.Handle("GET /api/log", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))
	mux.Handle("GET /api/log/", middleware.UserAuth(http.HandlerFunc(controller.ListLogs)))
	mux.Handle("GET /api/log/{trace}", middleware.UserAuth(http.HandlerFunc(controller.GetOwnedLog)))
	mux.Handle("GET /api/playground/results/{trace}", middleware.UserAuth(http.HandlerFunc(controller.GetPlaygroundResult)))

	mux.Handle("GET /api/audit", middleware.AdminAuth(http.HandlerFunc(controller.ListAuditLogs)))
	mux.Handle("GET /api/audit/", middleware.AdminAuth(http.HandlerFunc(controller.ListAuditLogs)))

	mux.Handle("GET /api/metrics", middleware.AdminAuth(http.HandlerFunc(controller.GetMetrics)))
	mux.Handle("GET /api/admin/dashboard", middleware.AdminAuth(http.HandlerFunc(controller.GetOperatingDashboard)))
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
	// Keep the retired numeric route as a 404 instead of serving the SPA fallback.
	mux.Handle("GET /api/export/{id}/download", middleware.ExportDownloadAuth(http.HandlerFunc(controller.DownloadExport)))
	mux.Handle("GET /api/export/files/{token}/download", middleware.ExportDownloadAuth(http.HandlerFunc(controller.DownloadExport)))
	mux.Handle("GET /api/export/files/{token}/dictionary", middleware.ExportDownloadAuth(http.HandlerFunc(controller.DownloadExport)))

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
	mux.Handle("POST /v1/responses/compact", relayChain(dto.InboundOpenAI, controller.RelayResponses))
	mux.Handle("POST /v1/conversations", middleware.TokenConversationAuth(middleware.TokenRateLimit(http.HandlerFunc(controller.CreateResponseConversation))))
	mux.Handle("GET /v1/conversations/{id}", middleware.TokenConversationAuth(middleware.TokenRateLimit(http.HandlerFunc(controller.GetResponseConversation))))
	mux.Handle("DELETE /v1/conversations/{id}", middleware.TokenConversationAuth(middleware.TokenRateLimit(http.HandlerFunc(controller.GetResponseConversation))))
	mux.Handle("GET /v1/responses/{id}", middleware.TokenReadOnlyAuth(middleware.UserRateLimit(middleware.TokenRateLimit(http.HandlerFunc(controller.RetrieveResponse)))))
	mux.Handle("POST /v1/responses/{id}/cancel", middleware.TokenResponseControlAuth(middleware.UserRateLimit(middleware.TokenRateLimit(http.HandlerFunc(controller.CancelResponse)))))
	mux.Handle("DELETE /v1/responses/{id}", middleware.TokenResponseControlAuth(middleware.UserRateLimit(middleware.TokenRateLimit(http.HandlerFunc(controller.DeleteResponse)))))
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
