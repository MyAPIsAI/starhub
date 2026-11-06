# V08: Security Scan Report

Date: 2026-11-05

## 1. govulncheck Results

**Tool**: `govulncheck` (golang.org/x/vuln)

**Result**: 15 standard library vulnerabilities found. All are in Go 1.26.5 standard library and **fixed in Go 1.26.6**.

**Affected paths** (traced to StarHub code):
- `http.ListenAndServe` (main.go:65) — net/http vulnerabilities
- `http.Client.Do` (service/channel.go:122) — net/http IDNA vulnerability
- `http.Client.Post` (service/alert.go:117) — same
- `tls.Conn.Write` (relay/ws.go:306) — encoding/asn1 recursion
- `crypto/tls` (various relay paths) — TLS-related vulnerabilities

**4 additional vulnerabilities** in imported packages and 4 in required modules — but StarHub code does not call the vulnerable paths.

**Recommendation**: Upgrade Go toolchain to 1.26.6+ to resolve all 15 standard library vulnerabilities.

## 2. go vet Results

**Result**: Clean — zero issues across all 21 packages.

## 3. Sensitive Information Check

### API Key Storage
- Token.Key: **AES-256-GCM encrypted** at rest (P29 T43), `json:"-"` on struct
- Token.KeyHash: HMAC-SHA256 hash for lookup, `json:"-"` on struct
- Channel.Key: stored as text, but `Omit("key")` on list queries and `MaskKey()` on display
- No raw keys in log statements

### Password Storage
- User.Password: PBKDF2-SHA256 (210,000 iterations), `json:"-"` on struct
- No plaintext passwords in any API response, log, or export

### Verification Codes
- PhoneVerificationChallenge.CodeHash: HMAC-SHA256, `json:"-"`
- EmailVerificationChallenge.CodeHash: HMAC-SHA256, `json:"-"`
- No plaintext codes in logs

### Log Content
- Log table stores: model name, token counts, quota, channel ID, request ID
- Log table does NOT store: request body, response body, API keys, passwords
- AuditLog stores: method, path, status, IP, request ID — no secrets

### Export Content
- Requests CSV: includes Token ID (numeric) and Token Name, NOT the raw key
- Ledger CSV: includes trace ID, amount, order ID — no secrets
- Usage CSV: aggregates only

### Webhook/Alert Payloads
- AlertEvent: type, channel_id, message, severity, timestamp — no secrets

## 4. Conclusion

- **No sensitive information leakage** found in storage, logs, exports, or API responses
- **15 Go stdlib vulnerabilities** — all fixable by upgrading Go to 1.26.6+
- **go vet clean** across all packages
- **API keys encrypted at rest** (AES-256-GCM + HMAC hash index)
- **Passwords hashed** (PBKDF2-SHA256 with high iteration count)
- **Verification codes hashed** (HMAC-SHA256, never stored in plaintext)
