# StarHub

[English](README.md) | [简体中文](README_CN.md)

StarHub is an LLM API gateway written in Go. It aggregates multiple upstream channels behind a unified OpenAI / Claude-style API, and provides management for users, tokens, channels, quota and billing ratios.

> Current version: `v0.1.0` (pure Go, no CGO, single-binary deployment)

## Features

- **Multi-protocol relay (P3)**
  - **Any inbound protocol × any outbound channel**: requests to `/v1/chat/completions` (OpenAI format) and `/v1/messages` (Claude format) can be routed to any channel type. The unified intermediate representation (IR) in `relay/dto` handles bidirectional protocol conversion; when the channel speaks the same protocol as the client, the body passes through byte-faithfully
  - **Multiple channel types**: OpenAI-compatible (1), Claude (14), Gemini (25), AWS Bedrock (15 — requests signed with SigV4, streaming responses parsed from the AWS binary event stream). These cover the three typical differences between providers (different auth headers / different path styles / SigV4 signing). A new channel only needs to implement `relay/channel.ChatAdaptor` and register itself in `relay/relay_adaptor.go`
  - **Full endpoint family**: `/v1/embeddings`, `/v1/rerank`, `/v1/audio/speech` (TTS), `/v1/audio/transcriptions|translations` (STT, replayable multipart), `/v1/images/generations|edits` (per-image billing, streaming image SSE), `/v1/video/generations` (Sora-style task API), `/v1/realtime` (WebSocket, an RFC 6455 implementation with zero third-party dependencies)
  - **Task-style API (submit-and-poll)**: the `TaskAdaptor` interface has billing hooks built in (`ValidateRequestAndSetAction → EstimateBilling (pre-charge) → submit → poll ParseTaskResult → AdjustBillingOnComplete (terminal reconciliation)`). A background reaper finishes tasks nobody polled, and failed tasks are refunded in full
- **Multi-channel routing**: picks an available channel by group + model, with cross-protocol failover (when the first channel returns 5xx it switches to a channel speaking another protocol), and auto-disables a channel when upstream returns `401/403`
- **Quota and billing**
  - Pre-charge + refund on failure + settle against actual usage afterwards; this three-stage model applies to text / images / tasks / Realtime alike
  - Group ratio, model ratio and completion ratio are all dynamically configurable
  - Conversion: `1 USD = 500000` quota units
- **Management API**: full CRUD for users / channels / tokens / options / logs
- **Permission model**: three roles — regular user (1), admin (10), root (100) — with both session auth and Bearer token auth
- **Zero-dependency deployment**: embedded pure-Go SQLite (via glebarez/sqlite), no external database required

## Getting Started

### Requirements

- Go 1.26+

### Build and Run

```bash
# Clone the project
git clone <repo-url> starhub
cd starhub

# Run directly
go run .

# Or build a single binary
go build -o starhub .
./starhub
```

On startup the server listens on `:3000` by default, with the database file `starhub.db` in the working directory (tables are created and seed data written on first launch).

### Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `STARHUB_PORT` | `3000` | HTTP listen port |
| `STARHUB_DB` | `starhub.db` | SQLite database file path |
| `STARHUB_ROOT_PASSWORD` | `123456` | Initial password for the root user (**change this in production**) |
| `STARHUB_CHANNEL_TEST_INTERVAL` | `1800` | Background channel health-check interval, in seconds (`0` disables) |
| `STARHUB_TASK_POLL_INTERVAL` | `60` | Background task poll interval, in seconds (`0` disables) |

### First Login

A root account is created by default:

- Username: `root`
- Password: `123456` (or whatever `STARHUB_ROOT_PASSWORD` is set to)

## Usage Guide

### 1. Log In to the Management API

```bash
curl -X POST http://localhost:3000/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"username":"root","password":"123456"}'
```

On success:

- The response body carries a `session` field (valid for 7 days)
- An HttpOnly cookie named `session` is set

Subsequent management calls can carry the session via that cookie or via an `Authorization: Bearer <session>` header (see `middleware/session.go`).

### 2. Create an Upstream Channel (admin)

```bash
curl -X POST http://localhost:3000/api/channel \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <session>" \
  -d '{
    "name": "openai-main",
    "type": 1,
    "key": "sk-xxxx",
    "base_url": "https://api.openai.com",
    "models": "gpt-4o,gpt-4o-mini",
    "group": "default"
  }'
```

- `models` is a comma-separated model list
- `base_url` does not include the `/v1` path (the adaptor appends it); leave it empty to use each protocol's official default address

Supported channel types:

| type | Protocol | Auth / path style | Key format |
| --- | --- | --- | --- |
| `1` | OpenAI-compatible | `Authorization: Bearer`, standard paths such as `/v1/chat/completions` | API key (separate multiple keys with newlines) |
| `14` | Anthropic Claude | `x-api-key` + `anthropic-version`, `/v1/messages` | API key |
| `15` | AWS Bedrock | SigV4 signing, `/model/{modelId}/invoke[-with-response-stream]` | `accessKeyId\|secretAccessKey\|region` |
| `25` | Google Gemini | `x-goog-api-key`, `/v1beta/models/{model}:generateContent` | API key |

Any inbound protocol can route to any channel type: for example, a client sending Claude format (`/v1/messages`) asking for `gpt-4o-mini` gets the DTOs converted on the way out to an OpenAI channel, and the response converted back to Claude format (including streaming events and error envelopes). When the protocols match (Claude inbound → Claude channel) the body passes through byte-faithfully.

### 3. Create an API Token

```bash
curl -X POST http://localhost:3000/api/token \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <session>" \
  -d '{"name": "my-token"}'
```

The `key` field in the response is the usable secret (`sk-` prefix, 48 random characters). The default quota is `500000` ($1). Optional fields: `remain_quota`, `unlimited_quota`, `expired_time` (Unix timestamp), `group`.

### 4. Call the Relay API

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": false,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

Streaming example:

```bash
curl -N http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}'
```

Claude-format inbound example (`/v1/messages` can route to any channel type; the response stays in Claude format):

```bash
curl http://localhost:3000/v1/messages \
  -H "x-api-key: sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

Video task example (submit-and-poll, settled automatically at a terminal state):

```bash
curl -X POST http://localhost:3000/v1/video/generations \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"sora-2","prompt":"a cat surfing","seconds":8,"n":2}'

curl http://localhost:3000/v1/video/generations/<id> \
  -H "Authorization: Bearer sk-xxxx"
```

### 5. List Available Models

```bash
curl http://localhost:3000/v1/models \
  -H "Authorization: Bearer sk-xxxx"
```

## Management API Reference

| Method | Path | Privilege | Description |
| --- | --- | --- | --- |
| `POST` | `/api/user/login` | Public | Log in |
| `POST` | `/api/user/logout` | Logged-in user | Log out |
| `GET` | `/api/user/self` | Logged-in user | Current user info |
| `GET` / `PUT` | `/api/user` | Admin | List users / update a user |
| `POST` / `GET` / `PUT` / `DELETE` | `/api/channel` (`/{id}`) | Admin | Channel management (supports all four types: 1/14/15/25) |
| `POST` | `/api/channel/test/{id}` | Admin | Channel health check (the probe converts according to the channel's protocol) |
| `POST` / `GET` / `PUT` / `DELETE` | `/api/token` (`/{id}`) | Logged-in user | Token management |
| `GET` / `PUT` | `/api/option` | Admin | System options (including ratio configuration) |
| `GET` | `/api/log` | Logged-in user | Usage logs |

## Relay API Reference

| Method | Path | Inbound format | Description |
| --- | --- | --- | --- |
| `POST` | `/v1/chat/completions` | OpenAI | Chat completion (streaming / non-streaming) |
| `POST` | `/v1/messages` | Claude | Chat completion (streaming / non-streaming); the Claude SDK can talk to it directly |
| `POST` | `/v1/embeddings` | OpenAI | Vector embeddings |
| `POST` | `/v1/rerank` | OpenAI | Reranking (Cohere-compatible format) |
| `POST` | `/v1/audio/speech` | OpenAI | Text to speech (audio passed through) |
| `POST` | `/v1/audio/transcriptions` | multipart | Speech to text (form + file, replayable on retry) |
| `POST` | `/v1/audio/translations` | multipart | Speech translation |
| `POST` | `/v1/images/generations` | OpenAI | Text to image (billed per image, `stream` supports SSE) |
| `POST` | `/v1/images/edits` | multipart | Image to image (multipart replayed, `n` bounded the same as the JSON path) |
| `POST` | `/v1/video/generations` | OpenAI | Video task submission (Sora-style) |
| `GET` | `/v1/video/generations/{id}` | - | Video task polling (settles at a terminal state) |
| `GET` | `/v1/realtime?model=...` | WebSocket | Bidirectional Realtime session passthrough, settled from the `response.done` usage |
| `GET` | `/v1/models` | - | Model list |

Relay authentication accepts both `Authorization: Bearer sk-...` and `x-api-key: sk-...` (the Claude SDK uses the latter by default). Every relay endpoint shares the `TokenAuth → UserRateLimit → Distribute → Relay` chain and the three-stage billing model.

## Billing

Billing uses a three-stage fund-safety model (pre-charge → settle → refund):

```
Before   PreConsumeQuota          pre-charged from the tokenizer estimate of prompt
                                  + a completion budget of max_tokens×n
After    PostConsumeQuota         settled against the upstream's real usage (including
                                  cache / audio detail); over-charge refunded, shortfall collected
Failure  RefundPreConsumedQuota   the full pre-charge is returned
```

- **Pre-charge prevents concurrent overdraft**: the check-and-deduct happens inside a single write transaction (SQLite serializes writers, equivalent to a `lockForUpdate` row lock). A user with $1 of balance sending 10 requests at once cannot get all of them admitted
- **Settlement is atomic**: the difference adjustment, the `used_quota` accumulation on token / user / channel, and the usage log all commit in one transaction, so the books always reconcile
- **Pricing precedence**: expression pricing (`model_price_expr`) > per-call pricing (`model_price`) > token pricing (the ratio formula)
- Every float→int conversion goes through the saturating truncation in `common/quota_math.go` (guarding against an overflow wrapping into a negative number, i.e. giving money away); requests that were truncated or clamped leave a `quota_saturation` audit marker in the log

The token pricing formula lives in `relay/pricing.go`:

```
effective input  = text + cached × cache_ratio + input audio × audio_ratio
effective output = text × completion_ratio + output audio × audio_ratio
quota consumed   = (effective input + effective output) × model_ratio / 1,000,000 × 500,000 × group_ratio × other_ratio
```

That is, one million tokens at `ratio = 1` costs $1 (500000 quota units). `other_ratio` is the product of special multipliers (`relay.PriceData.AddOtherRatio`): the "size × quality × count" of an image and the "seconds × count" of a video task hang off the same chain, reusing the same clamping and auditing.

Options hot-updatable through `PUT /api/option`:

| Option key | Shape | Description |
| --- | --- | --- |
| `group_ratio` | `{"default":1}` | Group ratio |
| `model_ratio` | `{"gpt-4o":2.5}` | Model token ratio (falls back to `1.0` on a miss) |
| `completion_ratio` | `{"gpt-4o":4}` | Completion ratio (falls back to `4.0` on a miss) |
| `model_price` | `{"model":0.01}` | Per-call pricing (USD per call; once matched, token counts are ignored) |
| `model_price_expr` | `{"model":"<expr>"}` | Expression pricing (USD per call), supporting tiered / dynamic prices; syntax and safety constraints in `pkg/billingexpr/expr.md` |
| `cache_ratio` | `0.5` | Cached prompt token discount (OpenAI's convention is 0.5) |
| `audio_ratio` | `40` | Audio token multiplier |

### Request-Side Validation and Metering

- Chat: `n` (`1..10`) and `max_tokens` / `max_completion_tokens` / Claude `max_tokens` (`0..100000`) are rejected out of range (400) during **request validation**. The multiplier ceilings mean the pre-charge can never be inflated maliciously
- Images: `n` (`1..128`) shares one billing gate across the JSON and multipart paths (including uint wrap-around to negative values); a full-width `×` in a size is rejected outright with a hint to use `x`
- Tasks: `seconds` (`1..60`) and `n` (`1..10`) as above
- Streaming requests automatically inject `stream_options.include_usage`; when the upstream returns no usage, the gateway estimates from the SSE content it accumulated locally, so nothing goes unbilled
- The upstream usage sub-fields — `prompt_tokens_details.cached_tokens`, `completion_tokens_details.audio_tokens` and so on — take part in settlement, and each sub-field is clamped within the total (a lying usage cannot inflate the discount)
- When a request fails or the upstream returns 4xx/5xx, the pre-charged quota is refunded

### Billing Basis per Endpoint

| Endpoint | Pre-charge | Settlement |
| --- | --- | --- |
| chat / messages | prompt estimate + a max_tokens×n budget | upstream usage (including cache / audio detail), falling back to the local estimate |
| embeddings / rerank | input token estimate | the input tokens of the upstream usage |
| TTS | per-call price, or an input token estimate | audio responses carry no usage, so it is billed on input |
| STT | per-call price, or an audio duration estimate (heuristic on file size) | the `duration` from verbose_json (≈25 tokens/second); otherwise the estimate stands |
| images | per-call price × size multiplier × quality multiplier × n | the count the upstream actually returned (a malicious value outside [1,128] falls back to the requested one; streaming counts `completed` events) |
| video task | per-second price × seconds × n | terminal reconciliation: on success by the request parameters, half price on cancellation, full refund on failure |
| realtime | per-call price, or a conservative token budget | the `response.done` usage (including audio detail); otherwise estimated from the session text |

## Project Layout

```
starhub/
├── main.go                        # Entry point: load config, init database, register routes, start the task reaper
├── e2e_test.go                    # P0-P2 end-to-end integration tests (with a mock upstream)
├── e2e_p3_test.go                 # P3 end-to-end tests: protocol matrix / full endpoints / tasks / Realtime
├── common/                        # Constants, context keys, crypto, unified responses, saturating quota math, multipart parsing
├── controller/                    # HTTP handlers; the relay.go family carries the shared retry loop and per-mode flows
├── middleware/                    # Auth (UserAuth/AdminAuth/TokenAuth), sessions, channel routing (Distribute, multipart-aware)
├── model/                         # GORM data models (user/token/channel/ability/log/option/task) and transactional quota operations
├── relay/
│   ├── dto/                       # Unified intermediate representation (IR) + per-protocol wire DTOs and conversions (openai/claude/gemini/bedrock/image/audio/task)
│   ├── channel/                   # ChatAdaptor / TaskAdaptor interfaces and the four provider packages (openai/claude/gemini/aws)
│   ├── relay_adaptor.go           # Adaptor registration switch (wire new channels in here)
│   ├── chat_writer.go             # IR → client protocol response / stream serialization (outbound side of the P3.2 matrix)
│   ├── pricing.go / quota.go      # Pricing engine and three-stage quota (shared by every mode)
│   ├── image_pricing.go           # Per-image pricing (size×quality×count, with guardrails)
│   ├── task_billing.go            # TaskPricing → quota conversion
│   ├── ws.go / realtime.go        # RFC 6455 frame codec and Realtime session relay
│   └── validate.go / estimate.go  # Request-side validation and token estimation
├── pkg/billingexpr/               # Billing expression engine (tiered / dynamic prices; design constraints in expr.md)
├── service/                       # Channel health checks, task polling (reaper), disable policy
├── setting/                       # Ratio and price runtime configuration (in-memory + persisted to the database)
└── doc/                           # Requirements and architecture reference documents
```

### Relay Chain

```
Client ──TokenAuth(validate token/quota/group)──▶ UserRateLimit ──▶ Distribute(parse and cache the
      request body/model/stream; for multipart, cache the raw bytes and Content-Type)
      ──▶ Relay(pick a channel → pre-charge → retry loop: on 5xx/timeout/network error step down
              the priority ladder to another channel, skipping channels that don't support the mode;
              4xx error envelopes are converted per the inbound protocol and passed through to stop;
              settle on success, refund when everything failed)──▶ background health checks (auto-disable /
              re-enable channels) + task reaper (terminal settlement)
```

Middleware passes state through the request context (`CtxUsingGroup` → `CtxRequestBody`/`CtxRequestModel`/`CtxRequestContentType`). Channel selection and retries are done by the `relayFlow` in `controller/relay.go` (the trail `used_channels` records `1->2->3`); see the convention documented on `NewRouter` in `main.go`.

## Tests

```bash
go test ./...        # Everything (unit + e2e)
go test -v ./...     # Verbose output
```

Coverage includes crypto and key validation (`common`), quota settlement and image-pricing invariants (`relay`), protocol conversion (`relay/dto`), the SigV4 documented vectors and event-stream parsing (`relay/channel/aws`), WebSocket frame codec (`relay/ws`), ratio parsing (`setting`), the data layer (`model`), channel health rules (`service`), and the full relay chain (`e2e_test.go` / `e2e_p3_test.go`, with built-in mock upstreams for every protocol: OpenAI / Claude / Gemini / Bedrock / images / tasks / Realtime).

## License

No open-source license has been declared for this project; all rights are reserved by default.
