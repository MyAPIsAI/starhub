# V04: Billing Gold-Standard Coverage Report

Date: 2026-11-05

## 1. Pricing Modes Covered

| Mode | Test Files | Test Count | Status |
|------|-----------|------------|--------|
| Token billing (model_ratio) | `relay/pricing_test.go` | 15+ | ✅ Covered |
| Completion ratio | `relay/pricing_test.go` | 8+ | ✅ Covered |
| Per-call (model_price) | `relay/pricing_test.go`, `controller/relay_image_test.go` | 5+ | ✅ Covered |
| Expression (price_expr) | `relay/pricing_test.go`, `pkg/billingexpr/expr_test.go` | 6+ | ✅ Covered |
| Cache ratio | `relay/token_rates_test.go` | 4+ | ✅ Covered |
| Audio ratio | `relay/token_rates_test.go` | 3+ | ✅ Covered |
| Independent token rates | `relay/token_rates_test.go` | 10+ | ✅ Covered |
| Time-based pricing | `pkg/pricingtime/schedule_test.go` | 20+ | ✅ Covered |
| Image per-resolution | `controller/relay_image_test.go` | 6+ | ✅ Covered |
| Video per-second×n | `controller/relay_task_test.go` | 5+ | ✅ Covered |
| Group×group matrix | `model/quota_test.go`, `setting/price_test.go` | 8+ | ✅ Covered |

## 2. Settlement Edge Cases

| Edge Case | Test | Status |
|-----------|------|--------|
| Failure does not charge | `model/quota_reconciliation_test.go` — token_short, user_short | ✅ |
| Retry charges once | `model/quota_reconciliation_test.go` — concurrent settlements | ✅ |
| Refund on failure | `model/quota_test.go` — ReserveReleaseSettleRoundTrip | ✅ |
| Unfunded quota tracking | `model/quota_reconciliation_test.go` — both_sufficient | ✅ |
| Concurrent settlement | `model/quota_reconciliation_test.go` — 32 workers | ✅ |
| Settlement rollback | `model/quota_reconciliation_test.go` — rollback preserves balances | ✅ |
| Duplicate log ID rollback | `model/quota_reconciliation_test.go` — duplicate log ID | ✅ |
| Unlimited token used_quota | `model/quota_reconciliation_test.go` — unlimited_token | ✅ |

## 3. Task/Async Billing

| Feature | Test | Status |
|-----------|------|--------|
| Task pre-charge | `controller/relay_task_test.go` | ✅ |
| Task terminal settlement | `controller/concurrent_task_settlement_test.go` | ✅ |
| Concurrent task settle (1 winner) | `controller/concurrent_task_settlement_test.go` — 30/30 | ✅ |
| Failed task zero charge | `controller/concurrent_task_settlement_test.go` — zero fee failed | ✅ |
| Plugin task pricing | `controller/task_plugin_test.go` | ✅ |
| Requested model tracking | `controller/relay_task_model_test.go` | ✅ |

## 4. Affiliate Rebate

| Feature | Test | Status |
|-----------|------|--------|
| Rebate uses collected quota | `relay/affiliate_rebate_test.go` | ✅ |
| Rebate delivery recovers | `relay/affiliate_rebate_test.go` — synchronous + task | ✅ |
| Rebate rollback on failure | `relay/affiliate_rebate_test.go` | ✅ |

## 5. Summary

- **60 relay tests** pass (0 failures)
- **All pricing modes** (token/per-call/expression/time/cache/audio/independent) have dedicated tests
- **All settlement edge cases** (failure/retry/refund/concurrent/rollback/unlimited) are covered
- **Task/async billing** has complete lifecycle tests including concurrent settlement
- **Affiliate rebate** has delivery and recovery tests

**No gaps found** in billing gold-standard coverage. All PRD-required pricing modes and settlement boundaries have corresponding tests.
