# Legal publication, Cookie choices and mobile console

Scope: M6 UI-2.5 / UI-4; UI-AC-15, UI-AC-23, UI-AC-24; M7 ADM-AC-18. Updated 2026-11-09.

## Implemented behavior

- `/legal/terms`, `/legal/privacy`, `/legal/cookies` and `/legal/refund` share the public navigation and show the currently effective text for the selected language, version, effective date/time and explicit IANA timezone. The homepage links all four documents. Login/registration policy views use the same published source.
- Missing translations and unpublished texts show a dedicated not-yet-published state with a retry action. API failures show retry feedback. No demonstration legal copy is substituted, and Cookies/refund no longer fall through to the privacy template. Uploaded content is rendered as inert plain text with preserved line breaks; TXT and Markdown uploads do not execute HTML.
- Root uploads immutable document versions in the deployment profile timezone. Historical, current and scheduled status is calculated across the complete history before pagination. The history supports kind/language filters, 20 rows per page and inspection of the original content/timezone. Non-root staff cannot read or upload legal versions. Existing transactional audit records are retained.
- Public documents switch at the stored effective instant. Responses are `no-store` and expose only public fields. `now` and `next_effective_at` allow an already-open page to refresh at a known next boundary; the regular active-tab polling interval is at most 30 seconds. Focus/navigation also refresh. Newly scheduled versions are discovered by polling. Browser suspension/offline operation can delay an open tab; a new request always selects the version effective at server time.
- Deployment profiles support `cookie_consent` and `cookie_consent_mode` (`banner` or `dialog`, omitted means banner). The anonymous `/api/legal-policy` endpoint exposes only region/timezone/consent settings, never deployment payment configuration.
- Public-page prompts offer necessary-only storage. A local receipt scoped to region and consent version remembers that choice across refreshes; cross-tab updates are observed. Footer/Cookie-page settings can reopen it. The Cookie policy remains readable before choosing consent. Storage failures have explicit feedback; no optional cookies or analytics integration is introduced. Automatic prompts are excluded from authenticated workspaces and authentication forms.
- Console tables in overview, usage, request history, billing and service quality become labeled vertical records on mobile. Desktop tables retain their layout and actions. The shared component derives mobile labels from the same translated headings. Mobile metrics are single-column; long Key selectors and narrow headers no longer widen the page.
- Key lists, report breakdowns and both spend distributions, exports and deployment history have pagination. Existing request/billing/service-quality pagination remains. Filter changes reset report pages. Key detail dates and export expiry dates include their timezone.

## Verification

| Acceptance | Evidence from this implementation | Remaining release dependency |
| --- | --- | --- |
| UI-AC-23 | Go tests exercise all four kinds in zh/en/ar on December 10, immediately before regional January 1 midnight, and at the exact boundary; future content/internal metadata remain private. Twelve browser combinations exercise v2 to v3 without reopening the page, all four links, unavailable/error states and retry. | Approved legal texts and the production region profile. |
| ADM-AC-18 | Existing end-to-end publication/audit tests and added non-root denial checks pass. The real isolated browser backend verifies Root authentication, TXT upload, public reading, filters and multiple history pages in eight zh/en theme/viewport combinations. A 28-version backend test checks current status across page boundaries. | Approved content owners and the release schedule. |
| UI-AC-24 | Twelve browser combinations verify first-visit prompt, necessary-only choice, no cookies before/after the choice and no repeated prompt after refresh. Dialog/banner/disabled policy, policy reading before consent and keyboard focus restoration are covered. | Launch country and approved display rules; review any future optional tracking before enabling it. |
| UI-AC-15 | 144 page visits: 12 console routes x zh/en/ar x light/dark x 320/390 px. Real isolated accounts with 23 Keys verify pagination, detail/create dialogs and mobile navigation. Populated report fixtures verify mobile field labels, table fit, pagination, filter reset, spend-list pagination and error recovery. No page errors or page-level horizontal overflow. | Real payment/upstream integrations retain their previous independent acceptance dependencies. |

The 12 console routes are overview, Keys, connection guides, playground, service quality, usage, request history, billing, recharge, exports, settings and smart routing. Empty data on the isolated backend is not represented as a successful real upstream call or payment. Populated report data and injected retry failures are explicitly browser fixtures; accounts and Key operations use the actual isolated backend. Visual review included desktop/public pages, mobile legal/Root upload, Arabic mobile report cards and mobile navigation.

Commands (run from repository root, except npm commands in `web`):

- `go build ./...`, `go vet ./...`, `go test ./...`: passed.
- `npm run build`: passed; existing large-chunk warning remains.
- `npm run test:unit`: 85 files passed, 1,010 tests passed, four existing skips.
- `npm run check:i18n`: 3,699 complete zh/en/ar keys, placeholders and usage passed.
- `node web/tests/ui25-browser.mjs`: 12 combinations passed, no page errors.
- `node web/tests/ui25-admin.mjs`: eight combinations passed, no page errors.
- `node web/tests/ui25-mobile.mjs`: 144 page visits passed, no page errors.
- `node web/tests/ui25-live-smoke.mjs`: 12 anonymous legal-page checks passed on the running local preview; all currently report unpublished, with no page errors, no automatic Cookie prompt and protected endpoints returning 401.

Evidence is under `output/ui25/` (ignored, including isolated database/session files). Old `usage-browser.mjs` and `playground-browser.mjs` harnesses still try customer login for a staff account and fail setup with 401 under the prior staff/customer split. They were not counted as passing; the current mobile harness uses actual customer accounts. The broad legacy M7 browser script also encounters the earlier renamed reconciliation control; this task uses the focused legal upload/history harness above.

## Local preview and outstanding decisions

The local backend update preserves account data, credentials, encryption key, business records, startup configuration and the existing Vite process. Migration rehearsal and post-update comparison passed; transient monitoring tables/refresh fields are excluded explicitly in the evidence. Backup: `output/ui25/update/backup-20261109-161026`. Backend PID 11968 serves API port 3000; Vite PID 18992 remains on 5173. No test legal content was inserted in the user's database and no production Cookie flag was enabled.

The user confirmed that launch-country rules and approved legal text are not available and authorized completing functionality/tests first. The current local profile keeps automatic Cookie prompts disabled, and unpublished legal pages report their real state. Final legal acceptance requires the approved country/profile settings and all required English/Arabic texts, plus a decision on Chinese legal text. Arabic native review remains pending from the earlier language work. This is an implementation/test result, not approval of legal wording or a claim of production legal readiness. No commit or push was requested for this task.
