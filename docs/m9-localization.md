# M9 language selection and RTL

Scope: I18N-3.1–3.4 and I18N-AC-01–06, 12–13. Arabic native-language review is pending: on 2026-11-06 the user confirmed that no reviewer is available and requested completion of code and tests first. The implementation does not certify linguistic review or legal approval.

## Language policy and account persistence

- `GET /api/localization` exposes only the current deployment profile's default and enabled languages, without caching. The Arabic feature switch removes Arabic from the public/customer selection.
- Selection priority is the authenticated account's saved language, the device's explicit selection, the first supported browser language, then the region default. Browser subtags such as `ar-AE` and `zh-CN` are matched. A disabled saved language falls back to the region default. The default profile is English.
- Public pages, customer console and documentation support English, Chinese and Arabic. Staff routes (`/admin`, `/staff`, `/finance`) expose English and Chinese only, including staff login. Entering staff routes does not erase an Arabic public-page preference.
- `PUT /api/user/language` and `PUT /api/admin/auth/language` accept `{ "expected_user_id": 42, "language": "zh" }`. Existing portal authentication and CSRF protection apply. A different owner gets 409; an unsupported language gets 400. Only the authenticated account's `preferred_language` column changes. Account responses include that field.
- Failed saves retain the previous language and show a translated error. Responses belonging to a previous session are ignored. The region policy refreshes at startup, on window focus and once a minute.
- Announcement content remains independently editable in three languages even though the management interface is bilingual. Published legal content uses its existing language/version contract; legal approval and the decision on Chinese legal text remain external. API error messages remain English.

## Transactional mail

Password and email changes made through the guarded account-profile API create a `transactional_mails` outbox record in the same database transaction as the security event. Queue failure rolls back the change. The recipient and effective account language are frozen at commit; an email-change alert goes to the previous address. Accounts without an email address do not queue mail. Templates exist in English, Chinese and Arabic and contain no passwords or credentials.

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

`npm run check:i18n` validates key coverage, nonempty translations, matching interpolation names, references and unused keys, and rejects legacy inline translation calls. The migration helper is `web/tests/extract-inline-i18n.mjs`. CI runs the check alongside type checking, unit tests and production build. Arabic strings remain draft translations until a native reviewer records their review against the catalog revision. Product terminology changes should update the glossary and every affected catalog entry together.

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
| AC-12 | Two staff interface options, three announcement content fields, Arabic docs and English API error |
| AC-13 | Reversed chart tick positions, mirrored legend, unchanged vertical sort indicator, regional currency and reviewed screenshots |

Automated checks do not replace Arabic native review or approval of final legal and announcement content. CI wiring is implemented; a remote CI run is only claimed when that run is actually observed.

### Verified on 2026-11-06

- Go build, vet and full regression passed; the final run is recorded in `output/m9-go-tests-final.txt`. Local SMTP, retry across database reopen and rollback checks passed.
- Frontend production build and formatting passed; 84 test files passed with 1,002 tests and four existing skips. The catalog check passed for 3,493 keys.
- Nine browser groups passed. The independent comparison run matched all 28 baselines exactly, with no page errors; see `output/m9-browser/report.json` and `output/m9-browser-compare.txt`. All eight additional console routes also passed desktop/mobile viewport checks in both themes.
- The local backend was activated after a successful migration rehearsal and data/credential comparison. Evidence and backup live in `output/m9-update-20261106`. Live endpoint and clean-browser smoke checks passed. No remote CI, real-provider email delivery or Arabic native review is claimed.
