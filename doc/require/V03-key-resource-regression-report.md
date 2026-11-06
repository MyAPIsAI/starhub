# V03: Key & Resource Authorization Regression Report

Date: 2026-11-05

## 1. Key Lifecycle Tests

| Test | Coverage | Status |
|------|----------|--------|
| TestCreateTokenValidation | Token creation with valid/invalid inputs | ✅ Pass |
| TestMaskedTokenHidesKey | Key masking in list responses | ✅ Pass |
| TestDeleteTokenNotFound | Delete non-existent token | ✅ Pass |
| TestRegenerateToken | Key regeneration produces new key | ✅ Pass |
| TestTokenPasswordVerification* | Password gate for key reveal | ✅ Pass |
| TestTokenInsertAndQuota | Token creation + quota operations | ✅ Pass |
| TestOwnedToken* | Owned token CRUD with receipts | ✅ Pass |
| TestConcurrentOwnedTokenWrites | Concurrent write protection | ✅ Pass |

## 2. Resource Authorization Tests

| Test | Coverage | Status |
|------|----------|--------|
| TestResourceCatalogPersistence | Snapshot import + read | ✅ Pass |
| TestResourceCatalogAtomicRollback | Concurrent write single winner | ✅ Pass |
| TestResourceCatalogValidation | Invalid snapshot rejection | ✅ Pass |
| TestResourcePublication* | Publication commit + conflict | ✅ Pass |
| TestResourceBindingChange* | Binding revoke + release | ✅ Pass |
| TestResourceGatewayPolicy* | Managed route policy | ✅ Pass |
| TestResourceGatewayInvalidation* | Channel identity change revokes | ✅ Pass |
| TestResourceRegionSave/Delete | Region CRUD with use protection | ✅ Pass |

## 3. Group Access Tests

| Test | Coverage | Status |
|------|----------|--------|
| TestServiceGroupAccess* | Group access enforcement | ✅ Pass |
| TestServiceGroupConfiguration* | Group definition CRUD | ✅ Pass |
| TestDeletedGroupCannotReopen | Deleted group stale ratio protection | ✅ Pass |

## 4. Encryption Verification

| Check | Status |
|-------|--------|
| Token.Key is AES-256-GCM encrypted | ✅ Verified by T43 tests |
| Token.KeyHash used for lookup | ✅ Verified by T43 tests |
| PlaintextKey() decrypts on demand | ✅ Verified by T43 tests |
| GetTokenByKey uses hash lookup | ✅ Verified by T43 tests |

## 5. Summary

- All Key lifecycle tests pass (creation, masking, deletion, regeneration, password gate)
- All resource authorization tests pass (catalog, publication, binding, policy, invalidation)
- All group access tests pass (access, configuration, stale protection)
- Encryption at rest verified (AES-256-GCM + HMAC hash index)

**No gaps found** in Key and resource authorization regression. Existing tests cover the full lifecycle including edge cases.
