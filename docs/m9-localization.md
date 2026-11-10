# M9 language selection and RTL

Scope: I18N-3.1–3.4 and I18N-AC-01–06, 09, 12–13. Arabic native-language review is pending: on 2026-11-06 the user confirmed that no reviewer is available and requested completion of code and tests first. The implementation does not certify linguistic review or legal approval.

## Language policy and account persistence

- `GET /api/localization` exposes only the current deployment profile's default and enabled languages, without caching. The Arabic feature switch removes Arabic from the public/customer selection.
- Selection priority is the authenticated account's saved language, the device's explicit selection, the first supported browser language, then the region default. Browser subtags such as `ar-AE` and `zh-CN` are matched. A disabled Arabic preference always falls back to English, including profiles whose default is Chinese. English remains selectable when Arabic is unavailable, even if the stored profile omitted English. Other unsupported preferences and new visitors use the region default. The default profile is English.
- Public pages, customer console and documentation support English, Chinese and Arabic. Staff routes (`/admin`, `/staff`, `/finance`) expose English and Chinese only, including staff login. Entering staff routes does not erase an Arabic public-page preference.
- `PUT /api/user/language` and `PUT /api/admin/auth/language` accept `{ "expected_user_id": 42, "language": "zh" }`. Existing portal authentication and CSRF protection apply. A different owner gets 409; an unsupported language gets 400. Only the authenticated account's `preferred_language` column changes. Account responses include that field.
- Failed saves retain the previous language and show a translated error. Responses belonging to a previous session are ignored. The region policy refreshes at startup, on window focus and once a minute. Opening the language menu waits for a fresh policy; a failed read keeps the menu closed and offers a translated retry message. Older policy responses cannot restore a disabled option. Disabling Arabic preserves the saved account/device preference so reenabling it restores Arabic automatically.
- Announcement content remains independently editable in three languages even though the management interface is bilingual. Published legal content uses its existing language/version contract; legal approval and the decision on Chinese legal text remain external. API error messages remain English.

## Transactional mail

Password and email changes made through the guarded account-profile API create a `transactional_mails` outbox record in the same database transaction as the security event. Queue failure rolls back the change. The recipient and effective account language are frozen at commit; an email-change alert goes to the previous address. Accounts without an email address do not queue mail. Templates exist in English, Chinese and Arabic and contain no passwords or credentials.

New mail for an account with a disabled Arabic preference uses English through the same profile policy as the language API. Already queued messages retain their frozen language.

Configure the deployment environment to enable delivery:

| Setting | Meaning |
| --- | --- |
| `STARHUB_SMTP_ADDRESS` | SMTP host and port, e.g. `smtp.example.test:587` |
| `STARHUB_SMTP_FROM` | Sender email address |
| `STARHUB_SMTP_USERNAME` | Optional SMTP username |
| `STARHUB_SMTP_PASSWORD` | Optional SMTP password; configure locally, never commit |

No SMTP configuration means no outgoing mail; pending messages remain durable. The worker checks every ten seconds, leases records, retries temporary failures after one minute and preserves pending work across restart. Successful messages are not deliberately resent. Delivery is at least once: a crash after SMTP acceptance but before the database acknowledgement can duplicate a message. A stable Message-ID aids provider deduplication. Remote SMTP requires STARTTLS with certificate verification and TLS 1.2 or later; plaintext is limited to loopback test servers. This task verifies a local SMTP fixture, not delivery through a real provider.

## Translation workflow

English is the source language. Shared keys are in `web/src/i18n/{en,zh,ar}.json`; extracted feature messages are in `messages.{en,zh,ar}.json`. `message` and `localizedMessage` reference catalog keys; the tuple adapter preserves existing component call signatures. `messagePair` is for bilingual management UI. The extraction script migrates legacy inline calls/tuples/conditionals; new text should be added directly to the catalogs. Interpolation names must match in every language. Model identifiers, API fields, code, vendor trademarks and user-authored content are not translated.

`npm run check:i18n` validates key coverage, nonempty English source and translations, matching interpolation names, references and unused keys. It rejects legacy inline translation calls, JSX copy, untranslated accessibility attributes and embedded language dictionaries/tuples. `web/tests/i18n-source-check.test.ts` covers these failure paths. Protocol names, brands, machine identifiers, multilingual search indexes, historical-value comparisons and the design system's Arabic glyph specimen are intentional data exceptions. Static checks do not assess translation meaning or dynamically supplied content. The migration helper is `web/tests/extract-inline-i18n.mjs`. CI runs the check alongside type checking, unit tests and production build. Arabic strings remain draft translations until a native reviewer records their review against the catalog revision. Product terminology changes should update the glossary and every affected catalog entry together.

For each copy change: add or update the English source key, translate Chinese and Arabic with matching interpolation variables, check the terminology table, run the catalog/unit/browser checks, and obtain native review before normal release approval. Record the reviewer, date, catalog commit or SHA-256, reviewed keys, corrections and product-owner sign-off in the [review record](m9-translation-review.md). The user's code-and-tests-first authorization does not mark that review complete.

Draft glossary for product/native review:

| English source | Chinese | Arabic draft |
| --- | --- | --- |
| Service group | 服务分组 | مجموعة الخدمة |
| API key | API 密钥 | مفتاح API |
| Model | 模型 | النموذج |
| Resource instance | 资源实例 | مثيل المورد |
| Balance | 余额 | الرصيد |
| Top up | 充值 | شحن الرصيد |
| Tokens | 词元 | الرموز |
| Usage | 用量 | الاستخدام |
| Trace ID | 追踪标识 | معرّف التتبّع |
| Success rate | 成功率 | معدل النجاح |

## RTL and verification

The document direction and Radix controls follow the selected language. Logical layout properties mirror navigation, forms, tables, drawers and dialogs. Numeric currency, code, keys, trace IDs, URLs and model identifiers remain LTR. Arabic dates use the regional locale with Western digits. Currency formatting preserves BigInt ledger precision, uses regional USD placement and bidi isolation; responsive report values do not split the symbol from the number. Time axes reverse in Arabic, the numeric axis and legend move to the mirrored side, and the descending request-time indicator remains vertical.

Run from the repository root:

```text
go build ./...
go vet ./...
go test ./...
```

Run from `web`:

```text
npm run check:i18n
npm run test:unit
npm run build
node tests/m9-browser.mjs
```

The browser suite starts its own backend, database and Vite server. It checks real customer/staff sessions and two-device language restoration; fixed report/log responses supply deterministic chart and drawer content. It verifies desktop/mobile, light/dark, Arabic console pages, docs, modals and drawers. Committed baselines live in `web/tests/baselines/m9`; current screenshots and the report go to `output/m9-browser`. Comparison allows a small pixel tolerance for rasterization differences. To intentionally regenerate locally, run with `M9_UPDATE_SCREENSHOTS=1`, inspect the images, then rerun without that flag. CI cannot update baselines. Browser fixtures disable external resource and SMTP endpoints and never insert data into the live database.

| Acceptance | Evidence |
| --- | --- |
| AC-01 / AC-02 | Fresh French, Arabic and Chinese browser contexts; regional fallback and RTL assertions |
| AC-03 | Real account update and second-device login; Chinese SMTP MIME test; durable mail retry/restart and transaction rollback tests |
| AC-04 / AC-05 | Console navigation matrix, RTL modal/drawer, LTR trace/code and exact currency tests |
| AC-06 | Catalog completeness, usage and interpolation check in CI |
| AC-09 | Real Root feature switch; fresh menu policy without page reload; existing account and fresh-device English fallback with English/Chinese defaults; rejected Arabic writes; restored preference after reenabling; transactional mail and omitted-English policy tests |
| AC-12 | Two staff interface options, three announcement content fields, Arabic docs and English API error |
| AC-13 | Reversed chart tick positions, mirrored legend, unchanged vertical sort indicator, regional currency and reviewed screenshots |

Automated checks do not replace Arabic native review or approval of final legal and announcement content. CI wiring is implemented; a remote CI run is only claimed when that run is actually observed.

### Verified on 2026-11-06

- Go build, vet and full regression passed; the final run is recorded in `output/m9-go-tests-final.txt`. Local SMTP, retry across database reopen and rollback checks passed.
- Frontend production build and formatting passed; 84 test files passed with 1,002 tests and four existing skips. The catalog check passed for 3,493 keys.
- Nine browser groups passed. The independent comparison run matched all 28 baselines exactly, with no page errors; see `output/m9-browser/report.json` and `output/m9-browser-compare.txt`. All eight additional console routes also passed desktop/mobile viewport checks in both themes.
- The local backend was activated after a successful migration rehearsal and data/credential comparison. Evidence and backup live in `output/m9-update-20261106`. Live endpoint and clean-browser smoke checks passed. No remote CI, real-provider email delivery or Arabic native review is claimed.

### Follow-up on 2026-11-10

The follow-up fixes the fixed-English fallback, consolidates mail policy, refreshes policy before opening the selector, and migrates overlooked homepage/group-picker/routing/pricing/export copy into the shared catalogs. The two mobile request-history screenshot baselines now represent the previously implemented responsive cards instead of the obsolete table layout; the pixel tolerance is unchanged. Current verification evidence is under `output/i18n-fix-20261110/` and `output/m9-browser/`. Remote CI and native Arabic review remain distinct from local automated verification.

Go build, vet and the full regression passed. All 87 frontend test files passed (1,023 tests and four existing skips); production build and the 3,921-key check passed. The real-backend browser suite passed ten acceptance groups and all 28 screenshot comparisons, including live-menu refresh, fresh-device fallback and restoring Arabic after reenabling it. Current Arabic routing desktop/mobile views were visually inspected. The local backend was updated after backup and successful rehearsal; post-update business-data, credential and encryption-key preservation checks passed. The frontend process was preserved.
