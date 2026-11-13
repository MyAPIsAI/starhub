# Key quota periods and verified secret reads

Key policies support `never` (a lifetime allowance), `daily`, and `monthly`.
Quota amounts are integer units: 500,000 units equal USD 1. The console converts
these to integer nano-USD strings before formatting amounts.

## Calendar periods and accounting

- Set `quota_period`, `period_quota_limit`, and `quota_region` when creating or
  editing a periodic key. Periodic limits are required and cannot be unlimited.
  `GET /api/token/quota-regions` lists enabled region clocks; an empty region
  selects UTC. The region's timezone is snapshotted into the key policy.
- Calendar boundaries use the selected timezone, including daylight saving
  changes. Monthly periods begin on the first day of the local month. A minute
  worker, authentication, reservations, and key reads refresh expired periods.
  Requests do not depend on the background worker running exactly at midnight.
- `TokenQuotaPeriod` retains each activated period's limit, settled charges,
  reserved quota, carried usage, and boundaries. Empty periods during inactivity
  are skipped. Lifetime `used_quota` is never cleared by rollover.
- Every production admission receives a server-generated durable
  `QuotaReservation` identifier. Top-ups, refunds, synchronous settlements,
  durable settlement retries, and asynchronous task completion retain that
  identifier. A request crossing midnight settles against its original period;
  a late refund cannot add quota to a new period.
- Available quota excludes pending reservations. Settled charges and pending
  reservations are displayed separately. Settlements use one transaction for
  receipt completion, quota counters, history, and the billing log. Replayed
  settlement intents or task completion cannot charge twice.
- Changing a period or timezone requires pending requests and settlement intents
  to finish. Switching schedules carries current spending into the replacement
  period without adding it again to lifetime spending. A limit cannot be reduced
  below current spending plus held quota. Guarded key edits retain their existing
  revision checks.

Existing keys migrate as lifetime policies and keep their balances. The existing
single-instance SQLite deployment model is retained. Pending admissions survive
restart. If an upstream outcome is unknown and no durable settlement result was
recorded, its hold remains reserved for reconciliation; restart does not blindly
refund or replay the upstream operation.

## Owned usage details

`GET /api/token/{id}/usage?period_page=1&model_page=1` requires the owner's active
session, including when the caller is an administrator. Responses are private and
not cacheable. They contain:

- Current settled usage, limit, reserved and available amounts, lifetime usage,
  calendar boundaries, and timezone.
- Thirty regional calendar days, including today, with explicit zero days and
  request, monetary, token, and outcome totals.
- Model aggregates for the same thirty-day window and all activated period
  records, independently paginated in pages of twenty.

Daily grouping uses admission timestamps. Older logs without that timestamp use
their completion timestamp. Late settlement can update a historical period; a
request admitted outside the thirty-day window remains in period and lifetime
accounting but is excluded from that window. Schedule-change carry amounts are
shown separately in history and do not duplicate model or lifetime charges.

The console refreshes key and usage snapshots every thirty seconds. Loading and
failed reads are distinct from confirmed zero usage. Usage caches and responses
are fenced by account and session identity.

## Repeated full-key access

`POST /api/token/{id}/unlock` accepts `expected_user_id` and the current password.
It grants at most five minutes of access to that key in the calling session.
`POST /api/token/{id}/reveal` accepts `expected_user_id`, checks the live grant,
decrypts the stored credential, and records an audit event before returning
`key` and `expires_at`. Both calls require the owner's active session.

Grants are bound to the user, session, password hash, and stored credential.
Password changes, logout, session expiry, key rotation, or key deactivation make
old grants unusable. A failed decryption or audit write never returns plaintext.
List and usage responses remain masked or credential-free; reveal responses use
`Cache-Control: private, no-store`. The browser keeps recovered plaintext only in
component state and removes it when the grant expires or identity changes.

## Verification

Run the repository checks with `go build ./...`, `go vet ./...`, and
`go test ./...`. Run `npm run test:unit` and `npm run build` in `web`.

`node tests/key-quota-browser.mjs` in `web` starts an isolated SQLite server and
local mock upstream, then exercises real login, key recovery, periodic creation,
policy editing, gateway billing, usage details, mobile layouts, accessibility,
and logout revocation. It uses Edge on Windows or Playwright Chromium elsewhere.
Masked screenshots and the report are written to the ignored directory
`web/playwright-report/key-quota`; temporary services and data are cleaned up.
