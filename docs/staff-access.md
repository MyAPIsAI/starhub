# Internal staff access (ADM-2)

Customers sign in at `/login`; internal staff sign in at `/admin/login`. Each form calls only its own authentication endpoint and never falls back to the other realm. Customer registration cannot create staff. A customer and employee may share a username, with independent passwords and different owner IDs.

Internal credentials, roles, status and authenticator state live in `staff_accounts`. Its primary key is the immutable ID of the employee's business owner row in `users`. That owner row has `account_kind=staff`, no password or authenticator secret, and cannot log in as a customer. Keeping this mapping preserves existing keys, balances, ledgers and audit actor IDs. Staff authentication always reads the dedicated table. All four staff roles have an owner-scoped personal workspace and the Platform management entry; management operations still enforce the assigned role.

| Role | Permitted work |
| --- | --- |
| Root | All staff management, role assignment and region profiles; all other management work |
| Operations | Customer and masked Key inspection/disable, model and pricing management, operations metrics, own audit records |
| Finance | Customer inspection, immediate manual balance adjustments, financial statements and reconciliation reports, operations metrics, own audit records |
| Technical | Resource/channel and routing inspection, forced full snapshot synchronization, system information, read-only customer/call inspection, own audit records |

Root sees all audit records. Non-Root staff cannot manage staff identities. Operations cannot mint redemption codes, upload task plugins, change non-pricing options, manually adjust balances or force upstream synchronization. Disabling a staff account or changing its role increments its security version and revokes existing sessions. Disabled identities remain available for historical audit references.

## Deployment initialization

For a commercial deployment, initialize the first Root from the deployment terminal:

```powershell
$env:STARHUB_DB = 'D:\deployment\starhub.db'
# Supply a temporary password through the deployment's secret mechanism.
starhub.exe staff init --username root --config D:\deployment\config.yml
```

The command reads the temporary password from `STARHUB_STAFF_PASSWORD`; passwords must be 12–72 bytes. It never prints passwords or API Keys. The server does not create a default-password Root. `STARHUB_ROOT_PASSWORD` is no longer a server bootstrap mechanism. Other staff accounts are created by Root in **Staff accounts**, with a role, temporary password and audit reason.

For open-source installation, set `STARHUB_STAFF_SETUP_MODE=opensource` before startup. If no Root exists, startup prints a one-time installation token, valid for 15 minutes. Open `/admin/login`, select **First installation: create Root**, and enter that token. Token verification and initial Root creation are atomic. A consumed token cannot initialize a second Root. `starhub.exe staff install-token` can issue a replacement token in this mode while installation is incomplete.

## Network, sessions and two-factor authentication

Configure `security.staff_allowed_ips` in the startup YAML or the comma-separated `STARHUB_STAFF_ALLOWED_IPS` environment variable. IPv4, IPv6 and CIDRs are supported. The default is loopback only. Declare trusted reverse-proxy peers separately with `security.trusted_proxies` or `STARHUB_TRUSTED_PROXIES`; client-supplied `X-Forwarded-For` is ignored outside this trust boundary. TLS deployments must enable secure cookies.

Every first staff login requires changing the temporary password and enrolling a time-based authenticator. Subsequent logins require a password and a new six-digit authenticator code. Password-only sessions expire after five minutes and cannot access protected staff APIs. Verified sessions last eight hours from verification; refreshing pages or switching between the personal workspace and back office does not extend that deadline or require another code. Signing out, expiry, a server restart (sessions are in memory), or a password/role/status/2FA change requires authentication again. There is no trusted-device bypass. Authenticator seeds are encrypted with the existing database credential-encryption key; preserve that key with database backups. Codes are single-use, with account-wide failure throttling. Enrollment seeds and credentials must not be copied into screenshots, logs or support reports.

Another Root can reset a lost authenticator after verifying the employee's identity through the company's recovery process. The action requires the acting Root's password, a fresh authenticator code and an audit reason, then revokes the target's sessions. A sole active Root uses:

```powershell
starhub.exe staff reset-2fa --username root --config D:\deployment\config.yml
```

The deployment command checks sole-Root status atomically and records the recovery. The target must enroll a new authenticator before returning to the back office. The last active Root cannot be disabled or demoted.

## Full resource synchronization

Set `STARHUB_RESOURCE_SNAPSHOT_URL` to the authoritative full-snapshot endpoint and, if required, `STARHUB_RESOURCE_SNAPSHOT_TOKEN` to its bearer credential. HTTPS is required outside loopback. Forced synchronization requests `GET ?full=true`, does not follow redirects, and validates the existing snapshot contract, including increasing sequence numbers. Fetch, validation or revision-conflict failures preserve the published snapshot. Without a configured source the API returns 503 and the UI disables synchronization; local option reloads are not reported as upstream synchronization.

## Upgrade behavior

The identity separation migration transaction copies existing staff password hashes, encrypted authenticator seeds, replay counters, lockout state and security versions into `staff_accounts`, then removes the credential values from staff owner rows in `users`. IDs and business records remain unchanged. Already-enrolled employees keep their passwords and authenticators. Repeating the migration never overwrites an existing staff identity. Older installations without account-kind classification still require the original first-login password change and enrollment.

Back up the database and its encryption key together before upgrading. Stop old server instances before starting the new version; old versions read credentials from `users` and cannot run concurrently with the split schema. Migration failure aborts startup. Rollback requires the matching pre-upgrade database/key backup, not only the old executable. Existing customer accounts cannot be promoted into staff through the retired finance-role endpoint.

Balance adjustments use a signed decimal USD amount, adjustment type, required reason and operation identifier. Finance/Root writes update the API balance, append an exact nanoUSD ledger row and record the actor, role, type and reason atomically. Duplicate operation identifiers cannot post twice. Deduction amounts may produce a negative balance.
