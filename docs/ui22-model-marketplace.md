# Public model marketplace (UI-2.2)

Implemented on 2026-11-09 against M6 UI-2.2 and UI-AC-01, UI-AC-02, UI-AC-03, UI-AC-18.

## Public data and billing

- `/api/catalog/models`, `/api/catalog/models/{id}` and `/api/catalog/filters` are anonymous endpoints. `/api/plaza/*` retains its account/group-specific contract for existing consumers. Model rankings are excluded by the overall PRD section 9.2 and the feature inventory; both `/api/catalog/rank` and `/api/plaza/rank` return 404.
- Public quotes use enabled common groups, ordered by multiplier and then stable group ID. Custom groups, account discounts and request-supplied group overrides do not affect public quotes. Zero is a valid free multiplier. Group definitions and ratios are dynamic.
- List prices come from the lowest available common group. Detail returns every enabled common group's own token, cache, audio or unit rates, conditions and published SLA metadata. Missing SLA remains unpublished; no target is synthesized.
- Prices reuse the gateway's authoritative pricing configuration, including time windows. In the AC-01 fixture, this base equals the official rate, so the result is official rate × multiplier. Separately maintained official reference metadata is not substituted for the billing configuration; source, date, historical flag and conditions remain reference information.
- Smart and conditional pricing have no advertised scalar price. Expressions include the public group multiplier; image formulas use the existing resolution/quality/count calculation. All actual calls remain priced and authorized by the selected key.
- Unlisted models return 404. Explicitly listed models retain detail URLs while temporarily hidden. Legacy models must have served a common group before appearing as public models; private-only legacy upstream identifiers are not published.

## Availability history

`catalog_availabilities` records actual availability, prior availability, outage start and the last supported channel types per model/group. Only transitions are written. A five-second worker observes published route coverage even without page visits; requests also observe current coverage. Persistence retains the outage start across process restarts.

- Newly unavailable models/groups have no grace period unless previously available.
- A continuous outage shorter than ten minutes retains normal display and configuration links. This is presentation history, never routing authorization.
- At ten minutes the model leaves the list if no common group remains available. Its detail still loads and labels the affected groups temporarily unavailable.
- Recovery resets the outage immediately at the next observation; the API refresh observes recovery directly. Browser queries refresh at least every minute and at earlier price-window boundaries.
- Unlisting clears historical visibility. Relisting without resources cannot inherit a prior grace period. Recreated group definitions cannot reuse the deleted definition's availability.
- Fresh gateway circuit reports can mark resources unavailable. Missing reports are unknown rather than fabricated outages. Existing publication, snapshot-expiry and upstream-identity guards still enforce actual routing.

## Presentation

The catalog follows the supplied `MyAPIs-Demo.html` layout: a compact heading and API-key shortcut, a bordered search/capability/vendor filter panel, separate count/view controls, and price cards with a highlighted platform-price column and a consistent detail action. Rankings, their hooks and endpoints have been removed. Required public pricing, filters, availability history and detail access are retained.

- Vendor tabs, capability filter, search, card/list selection, and pagination use URL state. The selected view also persists locally. Refresh and return from detail restore browsing state.
- Whole cards/rows navigate using real links. Text selection and copy buttons preserve their own behavior. Input/output prices can be copied with the model, group and unit.
- Detail includes the full group table, cache/audio prices, source and conditions, protocol metadata, published service targets, status summary and copyable cURL/Python/JavaScript examples. Examples use the demo/documentation address `https://api.myapis.ai/v1`; no external calls are performed by browsing or copying.
- Responses examples require explicit Responses capability. Other examples follow the published supported protocols. The gateway/key authorization rules remain decisive.
- MyAPIs Auto (`myapis-auto`) is pinned in the public directory and authorized console selectors, following the user's explicit platform-name correction. `veyra-auto` remains a compatible alias. Explicit saved requests are preserved and aliases do not grant new permissions.
- Phone layouts stack group prices into cards. Chinese, English, Arabic RTL and both themes are supported.
- The detail status summary uses actual customer call records, the highest-multiplier two common groups, a fifteen-minute window and a fifty-sample minimum. Classified platform/upstream failures enter the denominator; customer errors/cancellations and unclassified historical failures do not. Text TTFT uses successful inputs of at most 2,000 tokens. Missing samples/targets remain unknown. This is not an implementation of the separate UI-2.3 status-page configuration and history scope.

## Acceptance evidence

The later combined M6 review on 2026-11-09 found an unresolved UI-AC-25 boundary defect: when a cheaper common group loses its resources but remains within the ten-minute visibility grace, the public minimum still selects that group. A fixture with unavailable Lite and callable Standard reproduces this after five minutes. Price selection currently uses grace-adjusted visibility; it must use actual availability while keeping the existing visibility grace. The original UI-AC-03 fixture passes, but its shared pricing path is subject to this boundary defect. The current implementation is not certified as passing every M6 case.

The combined review also verified UI-AC-26 filtering/view restoration, UI-AC-27 price copying and complex-price presentation, and UI-AC-28 selected-group statistics. UI-AC-13 timed human sessions and UI-AC-14 PM review remain pending; legal/Cookie release settings remain unpublished or disabled as previously agreed. Supplemental reproduction and browser evidence are stored locally under `output/m6-acceptance-20261109/`, outside version control.

| Case | Verification | Result |
| --- | --- | --- |
| UI-AC-01 | Anonymous real-router requests; group prices; zero multiplier; identical public result with/without login; custom-group and private-model isolation | Passed |
| UI-AC-02 | Vendor + capability + search combinations and mismatches; browser filtering and restoration | Passed |
| UI-AC-03 | Lite without resources and Standard with resources; list uses Standard; detail preserves Lite and marks it unavailable | Passed |
| UI-AC-18 | Five-minute outage/recovery, second outage, exact ten-minute boundary, twelve-minute hiding, restart persistence, valid hidden detail, immediate recovery, unlist/relist | Passed |

Backend tests: `model/public_catalog_test.go`, `model/public_catalog_status_test.go`, `controller/public_catalog_test.go`, `e2e/public_marketplace_test.go`. The clock-driven tests do not wait ten real minutes or modify live business data.

Initial UI-2.2 evidence under `output/ui22/`:

- Go build/vet and full Go regression passed; socket-dependent tests ran outside the restricted local-network sandbox. Final availability/privacy changes passed the targeted model/controller/real-router suite (`acceptance-final.txt`).
- Production frontend build and translation validation passed (3,718 keys). Full frontend suite: 1,010 passed, four existing skips. The final grace-period presentation assertion also passed in `plaza-tests.txt`.
- `browser/report.json`: twelve independent combinations (zh/en/ar × light/dark × 1440/390), zero page errors. Checks cover whole-card navigation, price copying, filters/view after refresh/back, group tables, examples, keyboard interaction, minute refresh, hidden detail, empty/error states and page overflow. These browser fixtures are isolated from the live service.
- Current screenshots were visually inspected on desktop and mobile, including Arabic dark detail. Real anonymous page checks use the actual local backend and record their result in `update/live-smoke.json`; no mocked response, login or upstream call is used there.

## Demo refresh and MyAPIs branding verification

At the user's request, the ranking section and both ranking routes were removed, and the catalog was restyled from the supplied demo. The user's explicit MyAPIs name supersedes the earlier Veyra wording in the collaboration notes. Evidence is under `output/catalog-demo-refresh/`:

- Final frontend build and 3,711-key translation validation passed; the build retains the existing large-chunk warning. All 85 unit-test files passed: 1,010 passes and four existing skips. Existing card assertions were updated to the detail action.
- Go build, vet and the full Go suite passed. Real-router contract tests verify that both retired ranking endpoints return 404; smart-model aliases remain compatible.
- All twelve final browser combinations passed (zh/en/ar, light/dark, 1440/390), with zero page errors, no ranking requests and no page overflow. Filtering, view persistence, price copying, details and refresh still work. Current desktop, mobile and RTL screenshots were visually inspected.
- A separate clean-browser smoke used the real service, without fixtures or login. It verified the MyAPIs Auto name and identifier, the old detail alias, absent rankings and the real detail/group table. It did not make upstream calls.

## Local activation and boundaries

The tested backend is active on port 3000, with the existing Vite process on 5173 preserved. Database rehearsal, backup, integrity and original account/credential/business-data checks passed. Monitoring heartbeats and current alert values/timestamps are expected runtime updates and are explicitly separated in the preservation evidence. The final activation record and backup location are in `output/catalog-demo-refresh/update/activation.json`.

The real local data currently supplies two visible directory entries, including MyAPIs Auto. One pre-existing model description contains replacement characters in its stored metadata; this change does not invent replacement business content. Published SLA/source information still depends on operator-maintained data. Arabic native-language review and external production endpoint/resource-platform integration remain the previously recorded environment/manual dependencies. No commit or push was performed for this task.
