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

## Rollout state

The exact tariff calculator, schema, string projections, journal and reconstruction
checks are implemented. Integration of nano admissions, settlement, task recovery,
canonical wallet APIs and console consumers is the next rollout stage; the
presence of these tables alone does not enable exact charging or wallet linkage.
