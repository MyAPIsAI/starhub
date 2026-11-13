# Staff account security

Staff roles are Root (100), Admin (10), Finance (5), and Tech (7). Every staff
account must change its initial password, enroll an authenticator, and sign in
with both factors before using protected management APIs. This also applies to
existing accounts after upgrading and to accounts promoted from customer roles.
The server enforces this policy independently of the console.

## Installation

Initialize the first root account before starting the server:

```powershell
$env:STARHUB_OPERATOR_PASSWORD = Read-Host 'Temporary root password'
go run ./cmd/staff-account -db ./starhub.db -action initialize -username root -reason 'Initial installation'
Remove-Item Env:STARHUB_OPERATOR_PASSWORD
```

Prefer your secret manager or a protected environment when invoking the command.
The temporary password must contain letters and digits and be 8–64 UTF-8 bytes;
common weak passwords are rejected. The command reads it from the environment,
never from command-line arguments, and refuses to create another root account.
The initialized account still requires a different password at first login.

The existing server seed and setup API also produce accounts subject to this
security setup. Use a persistent `STARHUB_CRYPTO_SECRET` for credential storage.

## Console enrollment

1. Sign in with the initial account password. Only security setup, profile and
   logout operations are available during onboarding.
2. Choose a different strong password.
3. Confirm the new password and add the generated setup key to an authenticator.
4. Enter its six-digit code, save the displayed recovery codes securely, and
   select **Sign in again**.
5. Enter the password and then the authenticator code. The protected session is
   issued only after the server verifies both steps.

The password proof is a five-minute HttpOnly cookie, tied to the account,
password, factor, role, revocation generation, client address and user agent.
It is single-use: after an incorrect code, expiration, or a changed security
state, restart password verification. A TOTP code alone cannot create a session.
SMS login also requires the second factor when one is enrolled.

Enrolling a factor does not upgrade an existing password-only session. Old
sessions cannot acquire staff authority when an account is promoted. A changed
password, factor or role invalidates the staff assurance carried by a session.
An enabled factor cannot be overwritten through the setup endpoint.

## Operator recovery

Staff cannot disable their mandatory factor through the customer self-service
endpoint. An operator with access to the deployment database can reset it:

```powershell
go run ./cmd/staff-account -db ./starhub.db -action reset-2fa -username root -reason 'Approved authenticator recovery'
```

Initialization and reset atomically record the OS operator identity, target,
operation and required reason in the audit log. Audit failure rolls back the
operation. Reset removes the factor and its backup codes and advances the account
security generation. This revokes old sessions and unfinished login proofs,
including sessions that could access enrollment. The owner must authenticate
again, enroll a new factor, and complete a new two-factor sign-in.

The CLI is an operational trust boundary, not a remotely exposed recovery API.
Apply deployment access controls to the database and the command. No account
passwords, factor secrets, codes or cookies are written to audit records.

## Verification

Backend tests cover missing, reused, expired and changed login proofs, mandatory
staff enrollment, role promotion, atomic backup-code disable and audited recovery.
`node web/tests/staff-security-browser.mjs` exercises real console enrollment,
authenticator login, protected API access and recovery with an isolated SQLite
server. Set `STARHUB_TEST_BROWSER` to `chromium`, `firefox` or `webkit`; install the
selected Playwright browser first. On Windows the Chromium run uses Edge.
