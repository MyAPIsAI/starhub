# Credential storage and wrapping-key rotation

Customer API keys and upstream channel credentials are encrypted at rest.
The `enc:v2` format uses a fresh 256-bit data key for each value, AES-256-GCM
for the payload, and AES-256-GCM to wrap the data key. The version, wrapping-key
identifier and credential purpose are authenticated. A customer envelope cannot
be used as an upstream credential, or the reverse. Multi-key channel strings are
encrypted as a whole and split only after decryption in memory.

Channel struct reads/writes use a GORM field serializer. Map updates explicitly
encrypt the key field. Reads fail closed on a missing wrapping key, corrupted
value or incorrect purpose; ciphertext is never returned as usable plaintext.
Raw database access is reserved for migration and operational tools.

## Configuration

Keep `STARHUB_CRYPTO_SECRET` stable and secret across restarts. It remains the
customer-key lookup HMAC secret and the reader for pre-envelope encrypted data.
Changing the wrapping keys does not change customer API keys, lookup hashes,
channel IDs, resource ownership or active quota receipts.

Configure a small keyring through the deployment secret manager:

```text
STARHUB_CREDENTIAL_KEYS={"k2026a":"<64 hexadecimal characters>","k2026b":"<64 hexadecimal characters>"}
STARHUB_CREDENTIAL_ACTIVE_KEY=k2026b
```

Key identifiers contain 1–32 ASCII letters, digits, underscores or hyphens;
each key must decode to exactly 32 bytes. At most eight versions are accepted.
Invalid configuration fails closed. Secrets are not stored in source files,
command arguments, audit rows or application log messages.

When neither keyring variable is set, the `legacy` wrapping version uses the
existing `STARHUB_CRYPTO_SECRET` derivation for compatibility. Before switching
an existing installation to an explicit keyring, retain that version as an
entry named `legacy`: use the decoded bytes of a 64-character hexadecimal
`STARHUB_CRYPTO_SECRET`, or SHA-256 of its UTF-8 value otherwise. Add a new active
version alongside it. Without a persistent secret, encrypted credentials will
not survive a process restart; the random fallback is for disposable development.

## Legacy migration

Startup upgrades plaintext and old `enc:` customer/channel values in one
transaction and fills missing customer lookup hashes. Existing nonempty hashes
must match the stable lookup secret. It validates existing envelopes too.
An unreadable value, conflicting update or failed audit insert rolls back the
entire migration and prevents startup from serving partially migrated data.
The audit records the operation and reason, never credential material.

Use a protected backup before upgrading. Preserve the original lookup secret
and every wrapping version required by the database and retained backups.
Schema migration itself and the subsequent credential-data transaction are
separate operations; do not remove old secrets until the data migration passes.

## Rotation

1. Add the new version to the secret-manager keyring while retaining old versions.
   Make the complete ring available to the running service before writing new
   envelopes. Restart processes to apply deployment environment changes.
2. Select the new active version for the operator command and run:

   ```powershell
   go run ./cmd/credential-keys -db ./starhub.db -rotate -reason 'Scheduled wrapping-key rotation'
   ```

3. The command atomically rewraps customer and upstream values with the active
   key and records the OS operator identity and reason. If audit or a credential
   update fails, the credential-data transaction rolls back. The command prints
   counts only. Customer key lookup identity remains unchanged.
4. Set the service's active version to the new version, retaining the old readers
   until all writers have restarted. Verify the database with the intended final
   reader configuration:

   ```powershell
   go run ./cmd/credential-keys -db ./starhub.db
   ```

5. Retire old keys from active service configuration only after verification.
   Keep protected copies for any retained backups that still need them. Do not
   rotate `STARHUB_CRYPTO_SECRET` as part of wrapping-key rotation.

Use a maintenance window for large databases: migration and rotation serialize
SQLite writers. Operators need filesystem access to the deployment database;
the rotation command is not exposed as an HTTP endpoint.

## Verification

Tests cover randomized encryption, purpose binding, tampering, old-format reads,
missing/retired keys, struct and map channel writes, migration replay, atomic
audit failure and rotation without changing customer or channel identity.
