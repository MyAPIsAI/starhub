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

Smart entry compares input-plus-output unit prices rather than the current
prompt's estimated bill. `smart_candidates` is an optional comma-separated
subset on a token; it cannot expand ordinary model authorization. Stateless
chat and messages requests can fall back to another authorized compatible
model before committing a response. Additional quota is reserved before
sending a more expensive fallback. Headers and logs identify the actual model,
vendor, and fallback status; billing uses the serving model's captured price.
Stateful continuations retain their existing pinning behavior.

## Synchronization and diagnostics

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
provider-reported input counts from 1 to 2,000. Legacy aggregate and 15-minute
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

Relay handling never waits for telemetry HTTP delivery or SQLite writes. Failed
local persistence retries the same sealed batch; overflow marks its minute
incomplete. Unsupported calls, unsent attempts, uncertain outcomes and ordinary
customer validation errors do not become confirmed provider failures. Actual
transported attempts include protocol, endpoint capability, parsed requested
capabilities, resource generation, serving group, affinity outcome, usage and
timings. Client content and credentials must remain absent from these records.

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
