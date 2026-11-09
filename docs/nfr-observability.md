# NFR-6 observability and alerts

Implemented against `M10-非功能需求.md` NFR-6 and `M10-非功能需求.feature` NFR-AC-09/10. The staff presentation follows the supplied `MyAPIs-Demo.html` card/table/drawer design. Open `/admin?tab=observability` as Root or Technical; other staff roles and customer API keys cannot read operational telemetry. Staff call details link to the corresponding trace.

## Metrics and trace contract

- Gateway requests are recorded once, including rejection paths and fallback attempts. Metrics use completed requests in the preceding five minutes, grouped by protocol, model, vendor and service group. Success-rate eligibility excludes caller 4xx responses and client cancellation; failed streams with HTTP 200 count as failures. A retried request counts once. No eligible traffic produces an unknown rate, never a fabricated 100%.
- The latency columns are averages: first-token latency, total request duration, and gateway time after subtracting upstream attempts. Attempts include upstream transfer time; gateway time also includes local accounting and downstream overhead. System percentages are host CPU, physical memory and the working-directory filesystem, not container resource limits. Unavailable/stale samples display unknown.
- Precharge, settlement and ledger metrics report failed/total operations. Payment callback success rate uses the authenticated observation adapter below; the application does not yet have an agreed production payment callback integration. No samples means unknown.
- Snapshot age and source-version gap reuse the existing synchronization contract. Local routing counts use gateway reports; reports older than fifteen seconds are unknown. Gateway liveness uses a stable instance ID and a thirty-second heartbeat timeout.
- Each admitted API request receives a generated 64-character `X-Request-ID`. The relay forwards this ID upstream and records a safe provider request ID when returned. The trace endpoint joins gateway/authentication, precharge, channel selection and attempts, metering, settlement retries, deferred ledger posting and asynchronous task processing. Failed settlement remains pending or failed, never falsely posted. Rejected calls have a trace without a charge.
- `GET /api/admin/traces/{trace}` returns allowlisted metadata, call/settlement/ledger references and accounting status. Prompts, output content, request bodies, headers, passwords and keys are excluded. Trace events use JSON log records with a uniform trace ID and fixed event/status fields. Existing accounting records remain authoritative. Telemetry writes occur outside money transactions.
- Completed request telemetry is persisted when request handling finishes; an in-flight synchronous stream may not yet be queryable. Trace retention is thirty days; short-lived observation samples are retained for twenty-four hours. Accounting retention is unchanged. Up to 512 steps are returned per trace and up to 500 metric dimension combinations per snapshot.

## Alert rules and delivery

| Rule | Severity | Notification |
| --- | --- | --- |
| Platform five-minute success rate below 95%; unavailable gateway | Critical | Phone and IM, any time |
| Database unavailable; ledger or settlement write failure | Critical | Phone and IM, any time |
| Reconciliation difference; snapshot older than 180 seconds; two unaligned sync cycles | Serious | General endpoint during configured working hours |
| Elevated payment callback failures | Serious | General endpoint during configured working hours |
| Model/vendor success decline; CPU/memory/disk above 80% | General | General endpoint |

Rules run every five seconds. Only condition transitions enqueue generic notifications; recovery stays in history. Notifications are durable, claimed with a lease, retried with bounded exponential backoff and retained when endpoints are unconfigured. A recovery cannot overtake an undelivered firing notice for the same rule/channel. HTTP 2xx means the adapter accepted the notice, not that a person answered a phone. The receiver must deduplicate `Idempotency-Key` because a process can stop after remote acceptance but before local acknowledgement.

Existing reconciliation outbox rows are imported idempotently with their original `reconciliation-{id}` delivery identities. Reconciliation difference changes retain the existing notification behavior. The older channel-probe notifications remain compatible; they are separate from the five-minute request rules and are not their data source.

`GET /api/admin/observability` exposes metrics, freshness and configuration booleans, never endpoint URLs or tokens. `GET /api/admin/alerts?severity=critical&state=active&page=1` serves twenty alerts per page and the latest delivery records for those alerts. Empty severity/state means all. The UI shows pending/unconfigured delivery explicitly and hides stale success data on fetch errors.

## Configuration and adapter interfaces

Configure environment variables locally. Do not put real tokens into source, screenshots or reports.

| Variable | Purpose/default |
| --- | --- |
| `STARHUB_ALERT_PHONE_URL`, `STARHUB_ALERT_IM_URL` | Independent critical-notification adapters; initially empty |
| `STARHUB_ALERT_PHONE_TOKEN`, `STARHUB_ALERT_IM_TOKEN` | Optional bearer authentication for each adapter |
| `STARHUB_ALERT_GENERAL_URL`, `STARHUB_ALERT_GENERAL_TOKEN` | Serious/general adapter; falls back to IM, then existing `AlertWebhookURL` |
| `STARHUB_ALERT_TIMEZONE` | IANA timezone; initial default `UTC` |
| `STARHUB_ALERT_WORK_START`, `STARHUB_ALERT_WORK_END` | Weekday whole-hour window; initial default 09:00–18:00 |
| `STARHUB_ALERT_MODEL_SUCCESS` | Model/vendor success threshold, default `0.95` |
| `STARHUB_ALERT_PAYMENT_FAILURE` | Payment failure-rate threshold, default `0.05` |
| `STARHUB_ALERT_MIN_SAMPLES` | Minimum model/vendor/payment samples, default `20` |
| `STARHUB_OBSERVER_TOKEN` | At least 32 characters; protects health/payment observation interfaces |
| `STARHUB_GATEWAY_ID` | Stable, unique deployment identifier; default hostname plus listen port |

The model/vendor threshold, payment threshold, sample count and initial work hours are implementation defaults, not numerical values mandated by the PRD. The user has confirmed that provider interfaces, recipients and actual on-call arrangements are not determined. Configure those before production acceptance. Every replica must use a distinct stable gateway ID; intentional instance retirement also needs removal of that deployment's heartbeat registration to avoid treating it as a failed instance.

Adapters receive POST JSON containing `alert_id`, `rule`, `severity`, `transition` (`firing`/`resolved`), `occurred_at`, and applicable `scope`, `generation`, `value` and `threshold`. A separate `Idempotency-Key` identifies the delivery. The phone adapter is responsible for mapping the alert to real recipients and telephone calls; the IM adapter maps it to the chosen messaging provider. Endpoints require HTTPS except loopback HTTP for local tests. Redirects are not followed and request timeouts are five seconds. Adapter tokens and raw errors are not returned by staff APIs.

`POST /internal/observations/payment` requires `Authorization: Bearer <observer token>` and a strict JSON object:

```json
{"event_id":"callback-event-0001","source":"payment-provider","success":false,"at":1794038400}
```

`event_id` must be a stable 16–64 character event identifier; `source` must be a safe identifier; `at` must be within sixty seconds of server time. Use the actual current event time rather than the illustrative timestamp. Repeating identical data is idempotent; reusing an ID for different data returns 409. Unknown fields, trailing JSON and malformed values are rejected. This endpoint observes processing results and never credits or debits a wallet.

## Independent gateway/database watcher

Run the watcher as a separately supervised process, preferably on an independent host. It must not share the gateway's process lifetime or require its database to enqueue notifications:

```powershell
go build -o starhub-watch.exe ./cmd/starhub-watch
.\starhub-watch.exe --target https://gateway.example/internal/health --state D:\monitoring\starhub-watch.json
```

The authenticated health endpoint probes the database. The watcher checks every ten seconds, routes gateway/database failures to both critical adapters even outside working hours, stores a local durable outbox before delivery, and retries the same IDs after restart. Configure the observer token and notification endpoints in both process environments. Each watcher must have its own state file; run only one writer for each file. The staff configuration flag confirms only that the protected observer interface is enabled, not that an external watcher is deployed. No production watcher or external notification was enabled by the interface tests.

## Acceptance evidence

| Case | Automated evidence |
| --- | --- |
| NFR-AC-09 | Real five-minute request aggregates; expiry and reconciliation injection; severity/routing/work-hour checks; unknown-sample handling; persistent retry after DB restart; legacy queue import; firing/recovery ordering; independent gateway/database failure notification outside work hours |
| NFR-AC-10 | Actual gateway with failed-first/successful-second upstream; common trace ID and provider ID; metering/settlement/ledger join; rejected request without charge; no content/key in the returned trace; failed settlement rollback and retry without double charge; staff permissions |

Tests: `model/observability_test.go`, `model/observability_settlement_test.go`, `observability/watchdog_test.go`, `e2e/observability_acceptance_test.go`, `service/authz/observability_test.go`. Browser fixtures: `web/tests/observability-browser.mjs`, twelve language/theme/viewport combinations, filters, pagination, notification details, trace success/404, keyboard focus/scrolling, empty/error states and long IDs. Browser fixtures verify presentation; Go tests verify the real backend behavior. Current logs and screenshots are under `output/nfr6/`.

The code/interface acceptance is automated. Actual phone/IM delivery, recipient escalation, agreed business hours, production payment observation integration and independently supervised deployment remain environment acceptance items. Arabic native review remains the previously acknowledged separate localization dependency; the staff area intentionally falls back to English.
