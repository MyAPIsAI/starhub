# Exact monetary accounting

Canonical monetary values are signed integer nano-USD strings (`usd_nano`):
USD 1 equals 1,000,000,000 nano. Tariff computation uses rational token products
and decimal multipliers, followed by one half-up rounding at the nano boundary.
Positive tariffs have a one-nano minimum; explicitly free tariffs remain free.

## Compatibility and migration

Existing numeric quota fields retain their original scale of 500,000 ticks per
USD. Their exact storage representation adds a nano remainder to each balance
and counter: `nano = quota * 2000 + remainder`. Existing rows migrate with a zero
remainder. The migration does not multiply or reinterpret historical balances,
periods, task prices, reservations or pending settlement intents. Arithmetic
uses arbitrary-precision integers before checked decomposition, so a historical
int64 quota balance cannot overflow merely because nano-USD needs more digits.

JSON responses retain legacy numeric fields and add canonical string amounts.
Clients must use the string fields for exact accounting. Price snapshots retain
the exact product of special multipliers rather than a binary floating product.
Legacy snapshots retain their original version and unit semantics.

## Canonical journal

`MoneyAccount` is the head of a per-user nano-USD journal. `MoneyEntry` records
available, reserved and spent deltas, resulting balances, ordered sequence,
business operation ID, actor, reason, reservation and usage-log references.
Unique user/sequence and user/operation indexes prevent duplicate posting.
Balance projection, receipt updates, counters, usage log and journal posting
must share the same SQLite transaction.

An opening entry records existing spendable quota, lifetime usage and durable
in-flight holds. It is marked `legacy_quota_opening`; it is not a newly verified
payment. `VerifyMoneyLedger` rebuilds each balance from deltas and checks every
sequence and resulting snapshot. It reports drift without rewriting history or
silently promoting old unverified funding.

The legacy wallet and legacy ledger retain their provenance and monetary unit.
In particular, administrator quota adjustments use quota ticks while simulated
wallet entries use nano-USD. `LedgerMoney` normalizes their exact API/export
representation without rewriting source records or allowing simulated funds to
mint spendable quota.

## Active production contract

New relay, realtime, Responses and asynchronous task admissions use `n2:`
reservation IDs. Their stored reservation and frozen settlement/task amounts are
nano-USD. Native and plugin pricing snapshots use version 3; older IDs and
version 0-2 snapshots retain their original quota semantics. Public task JSON
keeps its legacy numeric projection and adds `quota_nano` and `amount_unit`.
The deprecated micro-USD fee is a projection; `exact_fee_nano_usd` is canonical.

The exact representation covers user, key, channel and period balances/counters,
usage and shortfall logs, reservation receipts, frozen synchronous settlement,
asynchronous task recovery, referral rebates and administrator-issued redemption
codes. Period rollover and policy changes keep fractional carry and assign late
settlements/refunds to the original period. A one-nano account and key can pay
for exactly one one-nano HTTP call. Explicitly free pricing remains free after
settlement; the existing nonzero admission-hold policy is retained.

## Wallet and funding APIs

- `GET /api/wallet/v2` returns the authenticated user's canonical available,
  reserved, total and spent nano amounts as strings.
- `GET /api/ledger/v2` returns paginated exact deltas, resulting balances,
  sequence, operation, reservation, log, actor and funding-source references.
  Other owners' ledgers require Root authorization and an explicit target.
- `POST /api/wallet/adjustments` accepts active Root grants/debits only. The
  actor, owner, signed nano amount, expected exact available balance, operation
  ID and reason are checked. Replaying the identical operation returns its
  receipt; a conflicting replay or stale balance is rejected. Balance, ledger
  and administrator audit commit together. It does not accept payment status or
  caller-supplied funding claims.
- `GET /api/wallet/reconciliation?user_id=...` requires Root and reports an
  account or reservation discrepancy as HTTP 409.

Legacy quota adjustments, redemption and referral credits share the canonical
journal. Existing spendable quota enters through a labelled legacy opening,
not a verified payment. Simulated top-ups still update only the separate legacy
payment wallet; they cannot mint API funds. Real payment callback verification,
refunds and chargebacks remain in deferred T31-T33. Account closure refuses even
one nano of available API funds or pending reservations; it never writes those
funds off through the legacy simulated-wallet dust policy.

## Reporting and verification

The console uses canonical funds, a paginated API ledger, nine-decimal key
budget inputs and exact usage/period strings. Monetary aggregates use arbitrary
precision before forming legacy display projections; SQLite integer SUM is not
used for money. Requests/usage CSV retain their legacy prefix and append exact
charged/unfunded nano columns. Export type `money_ledger` includes every monetary
dimension and audit reference; the console exports its last 31 days with session
and response-owner checks. Finance separates current API funds, recorded
reservations and legacy payment records; no field certifies real payments.
Upstream usage diagnostics retain the existing provider quota contract and do
not establish verified supplier costs or payment revenue.

The daily reconciliation stores exact log totals, usage snapshots, differences,
canonical sequence and ledger-verification status. The legacy daily snapshot
comparison remains diagnostic; `ledger_verified` is based on reconstructing the
canonical journal, checking the account/user projection, and matching reserved
funds to durable pending receipts. A one-nano discrepancy alerts without repair.

Verification includes mixed-unit migration, old in-flight data and restart,
concurrent reservations/grants, failed audit/log rollback, exact retries,
regional rollover, original-period late settlement, one-nano redemption/calls,
large aggregates beyond int64 and exact CSV. `web/tests/key-quota-browser.mjs`
uses isolated SQLite, real gateway APIs and a local provider fixture to check
wallet isolation, Root grants, key editing, usage, exports and finance across
English/Chinese/Arabic, desktop and narrow screens. This is local implementation
validation, not live payment, provider or deployment acceptance.
