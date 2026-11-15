# Single-instance production operations

This guide covers local implementation and deployment checks. It does not certify
production capacity, delivery to real recipients, RPO/RTO, or multi-instance support.

## Startup and traffic

Build the embedded console with `node scripts/build-release.mjs`. Set
`STARHUB_ENV=production`, a persistent `STARHUB_CRYPTO_SECRET` (at least 32
characters), and `STARHUB_ROOT_PASSWORD` (at least 12 characters). Keep the lookup
secret and every wrapping key needed by retained backups in a protected secret
manager. See [credential storage](CREDENTIAL_STORAGE.md).

Bind the server to a private interface or loopback behind a TLS reverse proxy.
Configure trusted proxy CIDRs, secure cookies, and management access restrictions.
Complete the initial password change and 2FA. Use one service replica.

`/api/live` reports process liveness; `/api/ready` also requires database access,
an initial commercial routing snapshot, and an accepting server. `/api/health`
retains its database-connectivity meaning. Configure probes outside customer
rate-limit budgets at the reverse proxy. A stale accepted snapshot follows the
existing conservative fallback policy; readiness does not promise supplier health.

On SIGTERM/interrupt the service stops accepting HTTP requests, allows up to 30
seconds for requests, cancels workers, waits up to 10 seconds, and attempts a
bounded telemetry checkpoint before closing the database. Allow at least 50
seconds before force-killing the process. Drain long streams at the proxy before
maintenance; streams exceeding this window and hijacked realtime connections may
still be interrupted. Durable settlement recovery remains necessary.

## Network databases and operator commands

Use `STARHUB_CONFIG` for the same YAML file as the server, or set
`STARHUB_DB_DRIVER` and `STARHUB_DB_DSN` together. Do not pass connection passwords
as command arguments. The network pool defaults to 25 open / 5 idle connections,
with a one-hour lifetime; tune against database capacity and pool wait metrics.
MySQL 8 requires `max_allowed_packet >= 65 MiB` (128 MiB recommended). PostgreSQL
has been validated on version 15. Configure authenticated encrypted database
connections according to the deployment trust boundary.

The operator commands read the same configuration and secrets:

```text
go run ./cmd/staff-account -action reset-2fa -username root -reason "Approved account recovery"
go run ./cmd/credential-keys -rotate -reason "Scheduled wrapping-key rotation"
go run ./cmd/reconcile-day -user 1 -date 2026-11-10 -timezone UTC -reason "Historical investigation"
```

SQLite can continue using `-db /existing/starhub.db`. Network drivers reject
`-db` so a mistaken path cannot silently select another database. Follow the
original admission calendar when reconciling historical dates. Use an isolated
maintenance window for schema migrations, large rotations and account recovery.

## Backups, restore and rollback

For application-managed SQLite snapshots set an existing private
`STARHUB_BACKUP_DIR`, `STARHUB_BACKUP_INTERVAL=1m` (or longer), and optionally
`STARHUB_BACKUP_KEEP` (default 7, maximum 10000). Snapshots run serially and start
at startup. A completed snapshot passes SQLite integrity checking before it is
published. Retention only removes managed snapshot names after a successful backup.
Monitor backup failure alerts and disk usage. Copy snapshots and required keys to
a separate protected failure domain. A one-minute schedule alone is not proof of
a one-minute RPO: snapshot duration and off-host transfer also matter.

```text
go run ./cmd/backup-db -db /existing/starhub.db -directory /protected/backups
go run ./cmd/backup-db -snapshot /protected/backups/snapshot.db
go run ./cmd/backup-db -snapshot /protected/backups/snapshot.db -restore-to /data/restored.db
```

Restore only immutable standalone snapshots. Existing destinations are never
overwritten. Stop the service before switching `STARHUB_DB` to the restored file;
preserve the old database and its WAL as a coherent set. Verify the recovered
ledger, quota receipts, routing state and telemetry watermarks in an isolated
environment before admitting customer traffic. Restoring an older snapshot may
lose later acknowledged business operations; never merge financial databases by
copying rows without an audited reconciliation plan.

MySQL/PostgreSQL use externally scheduled native backups; the HTTP snapshot API
returns 501 for those drivers. Configure database-native point-in-time recovery
where the recovery target requires it: MySQL consistent backups plus retained
binary logs, PostgreSQL base backups plus archived WAL. Logical dumps alone do
not prove a one-minute RPO. Store recovery credentials and keys separately, verify
restores to a new isolated database, then use the operator commands to inspect
funds and historical reports. Test against the actual server/version and retention.

Retain the previous executable/container, configuration, schema-compatible backup
and secret versions before upgrading. On failure first remove traffic and stop
the new process. Roll back binaries only when the schema/data remains compatible;
otherwise restore a tested database snapshot with its matching release/configuration.
Record elapsed recovery time, the newest recovered financial operation, and
unrecovered acknowledged operations. Actual deployment evidence is needed before
claiming RPO <= 1 minute, RTO <= 1 hour, or a 15-minute rollback.

## Message provider bridges

Configure `STARHUB_EMAIL_BRIDGE_URL` / `STARHUB_EMAIL_BRIDGE_TOKEN` and/or
`STARHUB_SMS_BRIDGE_URL` / `STARHUB_SMS_BRIDGE_TOKEN` for an operator-managed
provider adapter. URLs require HTTPS except for loopback HTTP. Redirects are not
followed. Requests have a ten-second transport timeout and Bearer authorization.
Unset bridges remain disabled; invalid configured bridges prevent startup.

The bridge accepts POST JSON with `kind` (`email_verification`, `sms_verification`,
or `security_notification`), `to`, and either `code` or `subject`/`body`. Queued
security messages also include a stable `id`; adapters must deduplicate it across
retries. A successful 2xx response must contain `{"accepted":true,"id":"provider-id"}`.
Do not acknowledge before accepting durable delivery responsibility. Providers
must protect codes and contacts and must not log payloads or authorization tokens.

Security notifications are queued atomically with password/email changes for
previously verified contacts. The original recipient and account/region language
are frozen. Ordinary email editing revokes verification. Failed delivery attempts
retry up to eight times with bounded backoff; disabled providers preserve pending
work. `/api/user/notifications?expected_user_id=1` exposes owner-bound acceptance
metadata without contacts. `accepted` means provider acceptance, not inbox delivery;
the adapter owns delivery retries, bounce handling and final delivery monitoring.
Investigate terminal failures before any audited manual requeue.

## Accounting and release acceptance

Minute-level reconciliation handles up to 128 changed business days per pass;
source changes and work revisions commit together. Startup and daily full audits
also inspect historical data to catch deletions/manual changes. An unchanged
report does not create repeated revisions. Current funding inputs retain separate
business receipts for credits/debits, redemptions and rebates. Old records without
independent evidence are reported as incomplete, never reconstructed as payments.

Before increasing traffic, validate real resource mTLS/port isolation, representative
supplier calls, every enabled business and operator path on the chosen database,
real metering shadow comparisons, and target load including accumulated history.
Observe accounting latency, database waits, notification failures, telemetry gaps,
backlog recovery and backup freshness. Existing deferred payment and multi-instance
features remain outside this release. External service acceptance and production
performance/recovery tests must be recorded by the deployment owner.
