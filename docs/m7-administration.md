# M7 administration

The administration implementation covers ADM-2, ADM-3, ADM-4, ADM-8 and ADM-9 and their fourteen originally requested acceptance cases. The subsequent operating dashboard implements ADM-5 with ADM-AC-07 verification. It uses the supplied MyAPIs Demo for page structure and the existing application theme. Customer and employee sign-in use separate forms and credentials, with server-confirmed role permissions.

## Personal workspace and back office

Every staff account has a personal workspace. Staff sign-in at `/admin/login` opens that workspace by default. A direct request to the back office returns there after sign-in. The Workspace menu links directly to personal pages, and Platform management returns to the back office with the account's assigned staff role.

The personal workspace sends `X-StarHub-Portal: workspace`. With a verified staff cookie, the server checks the current account, company network, 2FA and security version, uses the existing owner ID, and gives personal endpoints the ordinary user role. This keeps keys, billing and session management scoped to their owner. Staff credentials and account deactivation remain under staff administration. Administrative requests use the staff role. A successful switch between customer and staff logins revokes the other portal's session in that browser.

## Management pages

- Customer accounts: server-side search and paging, masked contact details, monthly spend, account details, concurrency controls, key disable, internal call metadata, ledger, orders and session revocation. Concurrent edits use an independent control revision; changing a concurrency limit does not revoke customer sessions.
- Models and pricing: stable model identity, explicit listing, resource eligibility, atomic base-price changes, reference guards on deletion, official reference provenance and separate pricing history. Existing advanced pricing engines remain available. Legacy configured models are shown as needing registration; migration does not silently unlist existing integrations.
- Region profile: Root-only versioned deployment settings, vendor and regional listing constraints, legal document versions and regional time. Current Key quota periods keep their original boundary when the regional timezone changes.
- System information: measured process heartbeats, build/configuration versions and database health. Technical and Root roles can access it.
- Reconciliation: daily regional comparisons of settled usage, API ledger and paid orders versus recharge credits, with exact nano-USD differences and separate funding sources. Finance and Root can read/run reports. Current-day reports are provisional. External payment statements and vendor invoices are not integrated by this change.
- Audit: actor, role, business action, object, available before/after summaries, reason, IP and time; role-scoped filters and a one-year retention window. Typed projections exclude passwords, authenticator seeds, complete API keys and upstream credentials.

## API contracts

All administration endpoints require an enrolled staff session and an allowed network address; authorization is enforced on the server.

| Endpoint | Access |
| --- | --- |
| `/api/admin/dashboard` | Operations, Finance, Root |
| `/api/admin/customers` and account details/activity | Staff roles with customer-read permission |
| `/api/admin/customers/{id}/control` | Operations, Root |
| `/api/admin/customers/{id}/sessions/revoke` | Operations, Root |
| `/api/admin/models` | Operations, Root |
| `/api/admin/pricing-history` | Operations, Root |
| `/api/admin/region-profile`, `/api/admin/legal` | Root |
| `/api/admin/system` | Technical, Root |
| `/api/admin/reconciliation` | Finance, Root |
| `/api/audit` | Own records; Root can select any actor |
| `/api/legal/{kind}?language=en` | Public, currently effective document only |

The audit page presents time (UTC), actor with role badge, action, object, a consolidated change summary, and IP address. Known action codes, fields and enum values are localized in Chinese, English and Arabic; identifiers and user-entered reasons retain their original values. Long summaries expand inline. Stored audit records remain unchanged.

`GET /api/audit` accepts `p`, `page_size`, `action`, `actor` (Root only, including `0` for the deployment terminal), `from` (inclusive Unix seconds), `until` (exclusive Unix seconds), and `q` (up to 200 Unicode characters). Search applies before pagination and treats SQL wildcard characters literally. Repeated `search_action` parameters let the frontend match localized action labels within the same authorized search scope. Responses add `object_name` for existing staff/customer accounts and access-key names, while preserving the captured object ID. All filters remain within the one-year retention window and the caller's existing visibility scope. The query additions require the rebuilt backend; no schema migration is introduced by this audit-page change.

New model writes include expected catalog, model and pricing revisions. A stale or invalid write leaves the published model, price and history unchanged. Official prices require a public source URL and an ISO verification date and never change settlement rates. Unlisted models are hidden from discovery and reject new calls with `model_not_found`; disabled customers receive `account_disabled`.

Admission routing evidence is frozen into both synchronous settlements and durable asynchronous tasks. Customer log responses omit internal resource, snapshot and affinity fields. Historic records without evidence remain explicitly unrecorded.

## Operating dashboard (ADM-5)

The dashboard at `/admin?tab=analytics` provides today, last seven days, last thirty days and custom inclusive calendar dates in the deployment timezone. Custom ranges are limited to one calendar year and cannot end after today. Seven cards show registered top-ups, settled consumption, completed calls, final success rate, distinct active customers, newly registered customers and distinct paying customers. The page also shows a service-group stacked trend, consumption distributions by group/vendor/call type/model, and the top twenty customers and models. The model distribution remains available even when there are fewer than twenty models. Account rankings link to customer details.

`GET /api/admin/dashboard?range=7d` accepts `today`, `7d`, `30d`, or `custom` with `start_date` and `end_date` in `YYYY-MM-DD` format. All metrics come from one read-only SQLite snapshot, scoped to customer accounts. Disabled customers retain their historical contribution; internal employee workspaces are excluded. The response uses decimal nano-USD strings, including sums beyond signed 64-bit limits. Frontend charts use floating-point values only for geometry; totals, shares and tooltip amounts derive from exact integers. No current-price multiplication or process-local counter is used.

Consumption and success metrics use persisted final call records. Retries within a call are not counted as extra calls; recorded failures and cancellations are included, pending asynchronous tasks are excluded. Requests rejected before a durable call record is created are outside this report. Active customers are distinct customer IDs with recorded calls in the period, regardless of their current enabled state. The source's frozen service-group/vendor/call-type dimensions are retained; missing historical dimensions are explicitly unrecorded. Model and group display names can reflect current catalog labels, without changing historical IDs or charges.

Top-up and paying-customer metrics currently count positive, explicitly registered offline receipts in the ledger. The existing online top-up flow is simulated and has no verified payment gateway integration. Simulated top-ups, historical credits without verified provenance and other staff credits (gifts/compensation) are separately reported under the funding definitions and excluded from registered top-ups. Offline receipts use the existing server-authored `offline_payment` adjustment prefix and staff-adjustment funding source. This dashboard does not add payment processing or rewrite historic funding provenance.

The page requests fresh data on entry, every five minutes while active, on focus/reconnection and on manual refresh. Each response reports its actual snapshot time, the regional window and the data cutoff. Empty periods show zero counts/amounts and no success-rate value. Errors hide stale results and provide retry. No schema migration is added by ADM-5; the new API requires the rebuilt backend.

ADM-AC-07 is covered by `model/operating_dashboard_test.go`: three actual receipt adjustments total $300, one thousand persisted final calls total $25, and queued API ledger charges reconcile exactly at the regional 11:06 snapshot. The test checks all distribution/trend sums, snapshot freshness, customer deduplication, unchanged repeat reads and visibility of newly persisted calls. Additional tests cover employee exclusion, funding provenance, disabled customers, precision beyond int64, DST and empty windows. Controller and real-router tests enforce role/2FA boundaries and date validation; the existing staff network guard also applies. Browser verification runs against a separate real backend and SQLite with synthetic test fixtures, never the user's live database.

Evidence: `output/operating-dashboard-go-tests.txt`, `output/operating-dashboard-unit-tests.txt`, and `output/operating-dashboard-browser/report.json`. Run the browser check with `node tests/operating-dashboard-browser.mjs` from `web/`; it verifies all twelve language/theme/viewport combinations plus range controls, refresh, drill-down, empty and failure states.

## Validation and deployment

Run `go build ./...`, `go vet ./...`, `go test ./...`, and the frontend unit/build checks. In environments where Git cannot read the checkout metadata, use `go build -buildvcs=false ./...`. Browser acceptance is `node web/tests/m7-admin-browser.mjs`; it starts isolated services and synthetic accounts. Run it from the repository root or adapt the working directory as required by the existing test tooling. The browser script uses its own temporary database and never enrolls a real account.

The migration adds tables and columns. Back up SQLite consistently together with its `.key` file. Preserve the deployment's database path, company network allowlist, snapshot source and other startup configuration. Restarting a process clears its in-memory login sessions, while stored passwords and authenticator bindings remain intact. Keep backups and test databases out of commits.

The original fourteen-case evidence is under `output/adm-m7-review-20261104`. `completion.html` maps those cases to executable evidence and records their local activation result; the later ADM-5 evidence is listed above. These checks do not certify incident publication, external provider deployment or the complete M9 regional feature rollout.
