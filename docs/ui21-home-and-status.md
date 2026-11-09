# Public homepage and service status

Scope: M6 UI-2.1 / UI-2.3; UI-AC-04, UI-AC-14 and UI-AC-17. Source: the supplied PRD and feature files in `C:/Users/Administrator/Desktop/自研平台/`.

## Published behavior

- The homepage keeps the MyAPIs visual identity and protocol illustration. The primary actions open registration and the documentation. Published models/vendors and enabled common-group descriptions come from `GET /api/public/home`. No sample spending, request counts, success rates, trend chart, testimonials or customer logos are rendered.
- The homepage example uses a published text model supporting OpenAI when available, otherwise an explicit `MODEL_ID` placeholder. Its base remains `https://api.myapis.ai/v1`, matching the user's Demo-address instruction. It requires a real key, authorization and a callable upstream; displaying or copying it does not validate production connectivity.
- `GET /api/status/models` is anonymous, returns only public model aggregates, and supports search, vendor/type/state filters and pagination (default 20, maximum 100). State filters use the same 15-minute decision as the displayed state and run before pagination across the matching catalog. Type facets describe the full published catalog. Unlisted/private-only models and raw customer, key, channel and trace data are excluded. Listed models remain visible during outages even when unavailable in the model marketplace. The private `/api/service-quality` route still requires authentication.
- The status page follows the supplied MyAPIs Demo: one bordered container per vendor, compact model rows, inline current state and 24-hour success/TTFT, type chips, search and All/Issues/Unconfirmed filters. It retains published incident announcements, refreshes every five minutes, and distinguishes loading, failure, no matches and insufficient samples. Zero success is displayed as 0%; missing measurements are a dash. The history defaults to 30 daily bars with a 90-day toggle. The PRD's P50 and P95 both remain visible.
- Daily bars open actual hourly history; a separate action opens the rolling last 24 hours. Arrow keys, Home/End and Enter operate the daily bars, and a date selector supports mobile and keyboard use. The detail dialog supports Escape and restores focus. Calendar days, hourly ranges and update timestamps use the deployment timezone with explicit offsets, including 23/25-hour DST dates.
- `GET /api/status/models/{id}/history` is anonymous and checks that the requested model is publicly listed. With no `date`, it returns rolling 24-hour buckets; an explicit `YYYY-MM-DD` must fall in the last 90 regional dates. Hidden models return 404, invalid dates return 400, and the response contains only public aggregates. Empty hours remain unknown; the UI supports independent detail loading, failure and retry.
- Under **Admin → Service groups → Edit definition → Public status statistics**, automatic mode selects the two enabled common groups with the highest configured multipliers. An operator can override each group to include or exclude it. Disabled groups never count. Existing configuration identity/revision/version guards and authorization remain in force. Older clients omitting the field preserve a stored choice.
- Explicit legacy ratio entries retain the same public/common meaning as the marketplace until a definition is registered. No synthetic group is inserted into the database. Deleted definitions, disabled definitions and private definitions are not silently converted to public groups.

## Statistical contract

| Value | Calculation |
| --- | --- |
| Current availability | Last 15 minutes, at least 50 eligible requests; below 95% is outage, 95% to below 99% is degraded, otherwise normal unless latency degrades it. |
| Success denominator | Successful calls plus classified platform/upstream failures under M1 4.1. Client errors, client cancellations, credential/model configuration errors and unclassified historical failures are excluded. |
| Current latency | Successful text calls with input at most 2,000 tokens and recorded positive TTFT; at least 50 latency samples. P95 above the widest published latency target among included groups degrades an otherwise normal state. |
| Last 24 hours | Actual final success ratio and nearest-rank TTFT P50/P95. No minimum sample gate; these display values do not determine current availability. Non-text models have no latency display. |
| Daily availability | Ninety calendar dates including the current partial day. At least 50 samples: at least 99.9% green, 99% to below 99.9% yellow, below 99% red. Fewer samples are gray. |
| Selected-period success | Total successful calls divided by total eligible samples across the displayed 30/90 dates, including gray dates with recorded calls. It is not the average of daily percentages. Missing numerators do not become fabricated zeroes. |
| Hourly success | Actual successful calls divided by eligible samples in each hourly bucket, and the same weighted calculation for the selected window. No minimum sample gate; an empty hour is unknown and 0% remains a measured failure rate. These display values do not determine current availability. |

The 24-hour and 90-day list aggregates are limited to requested, paged model names. State filtering calculates only the 15-minute decision for the matching catalog before pagination; hourly details calculate only the selected public model. SQL counts and latency frequencies avoid returning raw request records. All windows exclude future records and share the same group/failure rules. A lack of recorded history is not evidence of healthy service; missing days remain gray and are never filled with fabricated uptime.

## Homepage numerical-copy inventory

| Visible content | Source or interpretation |
| --- | --- |
| Model/vendor counts | Current published catalog response; vendor names are deduplicated. |
| Group/page counts | Current configured common groups and pagination arithmetic. |
| Model names and IDs | Published standard-model metadata or existing public legacy catalog entries. |
| Protocol paths such as `/v1/messages` | API identifiers in an explicitly labeled illustration, not statistics or customer endorsements. |
| “One key” | Product access model; not a customer-count or availability claim. |
| Example domain and JSON | The user's supplied Demo base; request structure is illustrative and includes no credential. |
| Capability descriptions | Existing key permissions, budgets/rate limits and recorded-call/report/ledger functions; no numerical SLA guarantee. |

## Acceptance and evidence

| Case | Result |
| --- | --- |
| UI-AC-04 | Automated pass: 45 successes + 5 upstream failures in the included groups produces outage, even with 500 successes in a low group. Anonymous real-router verification also passes. |
| UI-AC-17 | Automated pass: 40 included requests + 500 low-group calls + 30 client errors remains insufficient with 40 samples. Private-group traffic is also excluded by default. |
| UI-AC-14 | Implementation and three-language browser checks pass. **Formal acceptance remains pending the required pre-release PM review** of all homepage numbers, source records and copy. Automated checks are not PM written approval. |

Additional backend coverage checks 0% success, separate time windows, eligible latency samples, weighted percentiles, daily color thresholds, DST, explicit group overrides, disabled/deleted/legacy groups, stale-update rejection, invalid modes, anonymous access and public projections. Demo-refinement coverage adds state filtering before pagination, combined type/state filters, hourly privacy and date validation, rolling-window parity, 23/25-hour days, actual weighted ratios and missing-versus-zero behavior. Actual requests against isolated upstream servers verify that 429/5xx failures enter the denominator while 400/401/403/404 do not. Newly logged upstream 429 responses retain `RATE_LIMITED` instead of being collapsed into client errors; historical ambiguous logs are not rewritten.

Verification commands and evidence:

- `go build ./...`, `go vet ./...`, `go test ./...`; final output in `output/status-demo/go-tests.txt`.
- Frontend build, 3,765 translation keys and 86 unit files (1,012 passes, four existing skips); `output/status-demo/build-final.txt` and `unit.txt`.
- `node tests/ui21-browser.mjs`: 12 combinations of zh/en/ar, light/dark, 1440/390 pixels, plus 320-pixel overflow checks. Covers API-shaped fixtures, navigation, clipboard, pagination, type/state filters, 30/90-day switching, daily keyboard/date controls, hourly and rolling details, focus return, five-minute refresh, recovery, empty/error states and incident placement. `output/status-demo/browser/report.json` and screenshots explicitly contain synthetic fixtures.
- Prior UI-2.1/2.3 verification remains in `output/ui21/`: `node tests/ui21-admin.mjs` verified include/exclude/automatic saves against an isolated real backend, and `node tests/home-portal-browser.mjs` verified animation, scroll, keyboard, reduced-motion and responsive behavior. Those unchanged workflows were not rerun for this layout refinement.
- `node tests/ui21-live-smoke.mjs`: anonymous read-only homepage/status/hourly checks against the updated local backend/Vite in desktop Chinese and mobile Arabic, without fixtures or upstream requests. `output/status-demo/live/report.json` and current screenshots. The persisted catalog has one public model and no recorded calls; gray daily bars and unknown ratios are the expected real result.

Local backend activation uses a copied-database migration rehearsal, backup, known-process checks, health checks and business/account/credential preservation comparison in `output/status-demo/update/`. Backend PID 40776 is active on port 3000; Vite PID 18992 remains on 5173. Backup: `output/status-demo/update/backup-20261109-182115`. Existing calls and credentials are preserved; no fixture data was inserted. No commit or push is part of this task.

Existing external dependencies remain: real payment/upstream readiness, approved legal text and Arabic native review are not established by these changes.
