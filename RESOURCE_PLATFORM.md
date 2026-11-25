# Resource platform integration

StarHub consumes snapshots from the sibling `resource` service. It does not
generate commercial scores. This implementation targets one StarHub process
with SQLite; it does not introduce Redis, MySQL, distributed sessions, or shared
multi-replica circuit state. The receipt's `shared` flag means that the snapshot
was durably committed and installed for all request handlers in this process.

## Deployment and identity

Upgrade the gateway receiver before the resource publisher. Back up SQLite
before upgrading. Startup migrations add snapshot, credential, catalog, routing
policy, circuit, protection-outbox, and observation-work tables.

Configure the gateway environment:

| Variable | Meaning |
| --- | --- |
| `NODE_NAME` | Stable, unique deployment identity, 1–128 bytes; preserve across restarts. |
| `RESOURCE_CATALOG_NOTIFY_URL` | Resource service base URL, without credentials or query parameters. |
| `RESOURCE_CATALOG_NOTIFY_ENABLED` | Set `0` to pause catalog notifications only. |
| `METRIC_AUTHORIZATION` | Initial dedicated service token, without the `Bearer ` prefix. |
| `ROUTING_PROTECTION_SOURCE` | Source identity; default `talen`, matching the resource receiver. |
| `ROUTING_OBSERVATION_V2_ENABLED` | Default enabled; `0` pauses observation claims during upgrades. |

The resource client must send `Authorization: Bearer <token>` when calling
StarHub. The service token is independent of user API keys and administrator
sessions. The resource deployment may expect its authorization configuration
to contain the complete header value; configure each side accordingly.

Select `resource_provider_mode=commercial` through the existing option API.
Business endpoints return 503 with `Retry-After` until this process receives a
valid snapshot. Replaying the last accepted publication after restart can open
the gate, but does not reset the publication age. After three minutes, routing
uses priority and weight while preserving authorization, P/Q, capability,
endpoint-generation, and local protection checks.

For hard-failure stream consumption, configure the resource deployment with
`ROUTING_HARD_FAILURE_DETECT_ENABLED=1`,
`ROUTING_HARD_FAILURE_CONTRACT=attempt-http-v1`,
`ROUTING_HARD_FAILURE_LATENESS_SECONDS` in its supported 1–300 second range, and
`ROUTING_HARD_FAILURE_NODES` containing this gateway's `NODE_NAME` as a JSON
array. Execution is a separate resource-side decision controlled by
`ROUTING_HARD_FAILURE_EXECUTE_ENABLED`. A disabled or misconfigured receiver
causes queued events to remain pending and visible in routing status.

## Transport assessment (T10)

The current resource receiver authenticates internal protection and observation
traffic through trusted network placement and `X-Protection-Node`; it does not
verify application signatures. That header is an identity label, not a secret.
The gateway therefore requires an authenticated deployment boundary for these
outbound requests.

Use mutually authenticated reverse proxies or a service mesh on both sides.
Give the gateway and publisher separate certificates signed by a private CA,
verify certificate validity and the expected peer identity at each ingress,
and expose application ports only to those proxies. The gateway can address a
local proxy at an HTTP loopback URL; the proxy establishes mTLS to the remote
ingress. Keep the service bearer token on the gateway-facing routes as well.
Trust headers only after the proxy authenticates the peer, and redact
authorization headers and certificate private keys from logs.

Before deployment, verify trusted-client success and rejection of missing,
untrusted, expired, and wrong-identity certificates on both directions. Also
verify that direct access to the application ports is blocked. Certificates,
proxy configuration, and infrastructure are environment-specific; these live
checks have not been performed by the repository tests. Application signing
must not be claimed as a substitute for this deployment step.

## Routing administration

`GET` and `PUT /api/service-group/{group}/routing` use administrator
authentication. An example update body is:

```json
{"max_resource_ratio":"1.5","minimum_quality":"S1"}
```

The default is ratio `1` and quality `S2`. These admission constraints are
independent of the group's customer billing multiplier. Text candidates use
P ≤ G and the quality floor; non-text candidates omit the quality floor.
Snapshot pricing uses schema 4 `pricing.groups[].ratio`, not a scalar
`pricing.group_ratio` field.

Fresh text traffic uses the three best eligible resources at weights 70/25/5.
Affinity can reuse only a member of that pool and lasts at most ten minutes.
Retries can use lower-ranked resources, with at most three sends and a
60-second overall budget. Committed responses and uncertain side effects do
not get replayed. Non-text routing retains priority and weight after filtering.

Circuits persist in SQLite, open for at least 30 seconds, and back off to
300 seconds after failed half-open probes. Snapshot generation and recovery
epoch changes fence obsolete state. Slow resources leave the normal pool;
isolated probes allow recovery even when customer traffic never selects them.
Probes use platform credentials and do not create customer usage or charges.

EWMA baselines, degradation and recovery transitions publish versioned latency
evidence through the durable protection outbox. The resource receiver retains
these as per-node local signals, independently of its global protection mode.
Out-of-order reports cannot replace a newer signal, and a latency recovery can
never clear an operator disable. Inspect the receiver's authenticated
`GET /api/routing/latency-signals` endpoint with resource/node filters. Upgrade
the receiver before the gateway sender; the earlier receiver rejects these
additional causes. Unknown, expired and old-generation reports do not mutate
global protection epochs.

Smart entry compares input-plus-output unit prices rather than the current
prompt's estimated bill. `smart_candidates` is an optional comma-separated
subset on a token; it cannot expand ordinary model authorization. Stateless
chat and messages requests can fall back to another authorized compatible
model before committing a response. Additional quota is reserved before
sending a more expensive fallback. Headers and logs identify the actual model,
vendor, and fallback status; billing uses the serving model's captured price.
Stateful continuations retain their existing pinning behavior.

## Synchronization and diagnostics

### Restricted channel model append

`POST /api/resource-platform/channels/{id}/models` is the resource-service
write operation. It uses the dedicated service credential and accepts
`{"model":"..."}`. The operation atomically appends and deduplicates one model,
preserving the channel's existing models and groups, then creates abilities
for the channel's current groups. It cannot create channels or change
credentials. Model names are limited to 128 Unicode characters. Resource
Gateway managed channels are rejected because their published route records
require a separate workflow. The gateway mTLS egress allowlist admits this
POST route shape and separately allows `GET /api/channel/test/{id}` for the
service probe.

The resource service stores a separate entry ID and reuses it for repeated
submissions. It probes the specific channel/model pair and pulls the resulting
catalog through the existing synchronization path. Probe failure is reported
independently and does not disable the model; an enabled channel can route the
model under current admission rules. The service does not create channels or
handle provider credentials.

### Resource entry request and recovery contract

The sibling resource service writes through this idempotent POST, then probes
the exact channel/model pair through `GET /api/channel/test/{id}?model=...`.
The GET compatibility route is dispatched to the Resource Platform probe
handler and requires the dedicated service credential; it does not use the
admin-only browser test route and does not apply customer traffic isolation.
The append response and probe response fields are decoded by the resource
client. Resource persists probe work as pending/running/completed and recovers
interrupted probes from its database. Catalog import remains resource-side and
asynchronous after this StarHub append succeeds.

The channel identifier must be treated as a canonical decimal string by the
resource side because StarHub catalog exports numeric channel IDs, which the
resource importer formats in canonical decimal form. The resource service
normalizes parsed IDs before persistence and requests so entry lookup matches
the later catalog row. StarHub's append operation remains idempotent, allowing
the resource service to retry after an interrupted or uncertain request.
Resource and StarHub cannot share a database transaction; an absent HTTP
response can leave the caller uncertain whether the append committed. This is
handled by idempotent retry rather than claiming cross-service atomicity.

All `/api/metric/*`, catalog-export, and platform probe endpoints require the
dedicated service credential. The full and incremental catalog endpoints
retain 128 durable revisions; an unavailable revision returns HTTP 409 so
the publisher can pull a full catalog. Notifications are invalidation hints.
Endpoint generations change with channel identity, provider, URL, or key.

`GET /api/metric/routing/policy-status` reports the publisher-compatible
`data.instances` array, including rejected snapshot versions and errors.
`GET /api/resource/routing-status` requires administrator authentication and
reports snapshot age, fallback status, local circuits, pending event count,
recent event retry results, and recent observation work without result bodies.

Protection events are persisted before delivery. Hard-failure events are sent
in ordered batches of at most 64 once per second; failed batches retry with
the same IDs. A watermark cannot advance past an unfinished attempt. Restart
rotates the stream so interrupted requests do not become silently complete
evidence. Delivered events are retained seven days; pending events are retained
for retry. Monitor queue growth and receiver configuration before admitting
production traffic. The resource receiver's request-rate limit constrains
sustained event throughput; capacity testing remains a deployment requirement.

Observation v2 claims and results survive restart. An interrupted execution
reports uncertainty instead of calling the provider again. Transport failures
retry the stored result; explicit conflict or invalid-result responses retain
a terminal rejected work record. Claims, local probes, catalog notifications,
and event delivery run independently.

The model minute endpoint exports durable sealed production batches and complete
minute coverage manifests. The first partial minute after startup, open minutes,
collection loss, and unavailable intervals never claim complete zero traffic.
Generation changes remain separate partitions. Production counts and optional
health counters follow `confirmed-production-attempts-v2` and
`confirmed-production-health-v1`; TTFT requires successful first content and
provider-reported input counts within the requested input scope (2,000 by default).
`GET /api/metric/model/redis?ttft_input_cap_tokens=20000` requests a derived
20,000-token projection from the retained raw attempt journal. The supported
range is 1 through 1,048,576; resource sends its current policy's scope. Existing
sealed aggregates are never relabeled or rewritten. Legacy aggregate and 15-minute
projections remain process-local.

Metadata-only attempts are sent asynchronously to
`POST /internal/routing/attempt-metadata`, in batches of at most 64. Upgrade the
resource receiver before the gateway sender. Stable attempt IDs, exact receipt
validation, and durable retries prevent duplicate ingestion. Neither customer
identifiers, prompt hashes, request/response content nor credentials are included.
The sender and receiver retain at most 100,000 attempts for at most 24 hours;
the separate sealed minute aggregates retain 24 hours. The gateway also exposes
the sealed journal at `GET /api/metric/attempts`, protected by the dedicated
service credential. Bounded retention is not a permanent audit archive.

Attempt telemetry collection never waits for telemetry HTTP delivery or its
SQLite batch writer. Request logs and the independent request/decision audit
receipts are synchronous database writes. Failed
local persistence retries the same sealed batch; overflow marks its minute
incomplete. Unsupported calls, unsent attempts, uncertain outcomes and ordinary
customer validation errors do not become confirmed provider failures. Actual
transported attempts include protocol, endpoint capability, parsed requested
capabilities, resource generation, serving group, affinity outcome, usage and
timings. Client content and credentials must remain absent from these records.

The model-minute output retains schema 5 confirmed counts and health, and adds
schema 6 `single-input-ewma5-v1` speed evidence. Each scope is rebuilt only when
the complete sealed attempt journal is available. Events use the original
attempt ID, generation, completion timestamp converted from milliseconds to
microseconds, and raw first-content latency. Only successful streaming attempts
with positive provider-reported input counts within the selected scope qualify.
Missing journals, open minutes and generation/scope mismatches cannot certify
complete speed coverage.

## Effective catalog pricing

Catalog prices use `effective-pricing-v1`. Ordinary token models publish the
same effective defaults used by billing, including models without explicit
model/completion ratios. `unit_prices` contains decimal USD per million tokens;
`price_source` and `component_sources` distinguish default fallbacks from
configured prices. Resource reads these explicit units before considering any
legacy new-api ratio conversion.

The default input/output/cache-read prices are 1/4/0.5 USD per million tokens.
The ungrouped `text-500-500-v1` quote is 2.5 USD/Mtokens. A group ratio of 0.5
produces 1.25, applying that ratio once. Expressions remain context dependent;
positive per-call prices and independent token schedules retain their modes.
A zero flat-price option follows the existing runtime token-mode semantics.
Explicit free tariffs remain free; relative cost scoring with a zero reference
remains unavailable rather than inventing a division result.

Quote versions cover effective rates, units, modes, provenance and schedule
boundaries. Expiring quotes include a stable generation timestamp and expiry;
small positive decimals retain their exact scale. Historical settlements and
their frozen prices are not rewritten. Catalog invalidations update both the
resource catalog tariffs and model reference tariffs through normal sync.

## Event completeness and runtime populations

`GET /api/metric/events/completeness` uses the dedicated resource service
credential. Optional Unix-second `start_timestamp` and `end_timestamp` select
at most the retained 24-hour window; the open minute is excluded. The
`starhub-events-v1` response reports request, decision and attempt totals and
nullable percentages for finalized HTTP relay requests. Request starts have
independent receipts; selection calls have independent completion records;
sent attempts correlate internally with their request receipt. Customer
request IDs are not added to exported attempt metadata.

Pre-dispatch rejections require no upstream attempts. Empty populations,
historical requests without receipts, unresolved old requests, retention gaps
and unexpected duplicate events remain unknown. Audit receipts retain 25 hours
with bounded cleanup; they are not a permanent external reconciliation archive.

The response also contains an independent closed 30-minute runtime summary:
customer request success (including admission rejection), sent-request success,
smart-routing request success, request fallback rate, and actual channel
selection latency P95/P99 using nearest-rank percentiles. Each request counts
once even when it has multiple provider attempts. Resource retains its legacy
TTFT bucket statistic under `model_ttft_bucket_p95_max`, explicitly a maximum
of bucket P95s; it is never reported as routing decision time or global P95.
Failed remote reads preserve other summaries and their genuine observation
times. Unknown completeness does not become a fabricated 100 percent.

The default resource cost comparison and deployment cost metrics use enabled
resources bound to the current catalog version. `scope=file_reference` selects
the separately labeled full-file comparison. Billing benchmarks with channel
discounts are estimates; absent supplier settlement costs remain unknown.

Chat and native usage logs record provider-returned request IDs when available
and distinguish `upstream`, `upstream_adapted`, `estimated`, and `unknown` usage.
These fields are gateway owned and cannot be supplied through provider JSON.
Internal ledger consistency is not proof of an external supplier bill.

Run `powershell -File scripts/test-resource-contracts.ps1` with the sibling
resource checkout to generate isolated SQLite fixtures and replay the actual
catalog and speed contracts through the consumer, its price tables, and routing
snapshot pricing. The runner creates files below `tmp` and does not contact a
paid provider or modify a deployed service.

## Credential rotation

Root can call `PUT /api/resource-platform/credential` with
`{"token":"<new random token of at least 32 characters>"}`. The previous
credential remains valid for exactly ten minutes. Repeating the same new
credential does not extend that overlap. Update the publisher during the
window, then verify that its old token is rejected. Only SHA-256 digests are
stored. `{"disabled":true}` immediately revokes both credentials and prevents
fallback to the bootstrap environment value.

## Verification and remaining scope

Repository tests cover snapshot checksum/replay/restart gating, catalog
deletions and revision fallback, credential overlap, top-three distribution,
admission filters, affinity boundaries, stale fallback, circuit fencing and
probe backoff, EWMA recovery, three-send budgets, uncertain-result boundaries,
smart-model fallback billing, observation result retries, and event batch
ordering. Run `go build ./...`, `go vet ./...`, and `go test ./...`.

These tests use local fixtures and HTTP test servers. They do not certify a
live resource deployment, provider interoperability, mTLS infrastructure, load
capacity, or multi-instance consistency. Local score generation (T55) and
multi-instance sessions (T58) remain explicitly deferred by the requirements.

## Optional tested mTLS transport

[SERVICE_MTLS.md](SERVICE_MTLS.md) documents the ingress/egress sidecar,
operator-provided certificate identities, the private gateway bind address,
and the local certificate-rejection matrix. The sidecar preserves backend
service authentication. Its local tests do not complete T10/V01 deployment,
raw-port isolation, certificate provisioning, or real-provider acceptance.
