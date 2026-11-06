# V12: Open-Source Delivery Verification Report

Date: 2026-11-05

## 1. License Files

| File | Status | Content |
|------|--------|---------|
| `LICENSE` | ❌ Missing | No LICENSE file in repo root |
| `NOTICE` | ✅ Present | AGPL Section 7 additional terms (attribution, origin link, change marking) |
| `THIRD-PARTY-LICENSES.md` | ✅ Present | Lists Go and frontend dependencies with licenses |

**Action needed**: Add a LICENSE file (AGPL-3.0 full text or confirm current licensing).

## 2. Setup Wizard

**Implemented** (P33 T60):
- `GET /api/setup` — reports setup_needed and token_required
- `POST /api/setup` — creates first root user
- `STARHUB_SETUP_TOKEN` env var for production init protection
- Username/password validation aligned with PRD (2-24 chars, 8+ with letters+digits)
- Tests: `TestGetSetupNeeded`, `TestPostSetupCreatesRoot`, `TestPostSetupAlreadyCompleted`, `TestPostSetupValidation` — all pass

## 3. Build & Docker

| Component | Status | Location |
|-----------|--------|----------|
| Release build script | ✅ | `scripts/build-release.mjs` |
| Dockerfile | ✅ | `Dockerfile` |
| Go build | ✅ | `go build ./...` — clean |
| Frontend build | ✅ | `npm run build` — clean |
| Embedded console | ✅ | `//go:embed` in `web/` |

**Missing**: `Dockerfile` does not `COPY LICENSE NOTICE /licenses/` (Talen does this).

## 4. Minimum Configuration

**Not documented**. README should include minimum deployment specs:
- CPU: 1 core
- Memory: 512 MB
- Disk: 100 MB (SQLite + logs)
- Go: 1.26+
- No external dependencies (no Redis, no PostgreSQL)

## 5. Upgrade Path

**Not verified**. AutoMigrate handles schema additions (new tables/columns). Downgrade is not supported. Version is `0.1.0` (`common/config.go:61`).

## 6. Conclusion

- Setup wizard: ✅ implemented and tested
- NOTICE + THIRD-PARTY-LICENSES: ✅ present
- LICENSE file: ❌ missing — needs to be added
- Docker COPY of license files: ❌ missing in Dockerfile
- Minimum configuration docs: ❌ missing
- Build system: ✅ working (Go + frontend + Docker)
