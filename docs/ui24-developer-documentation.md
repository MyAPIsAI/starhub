# Developer documentation: UI-2.4 / UI-AC-13

## Scope and implementation

The public `/docs` route contains 22 articles in English, Chinese and Arabic. It retains authentication, service groups, protocols, Chat Completions, Responses, Anthropic Messages, native/compatible Gemini, model discovery, Claude Code, Cursor, errors, billing, key routing policies, limits and FAQ; it adds dedicated embeddings, images, audio, video, Gemini CLI and Codex articles. Each article has a copyable example.

Quickstart now covers registration, callable quota, key creation and model authorization, terminal setup, model discovery, an initial non-streaming request and its success/error criteria. Bash/cURL and Windows PowerShell are selectable; the official Python SDK is an alternative. The user enters a nonsecret model ID; the API key is entered only in their local terminal. At the user's request, the default API endpoint is `https://api.myapis.ai/v1`, matching the supplied Demo, with a validated override for separate gateways. Gemini native, Gemini CLI and Claude Code use the derived root `https://api.myapis.ai`. Invalid, credential-bearing or query-bearing endpoints disable copy. No real key or user funding is collected by the documentation page.

Examples use the same configurable endpoint, including protocol-specific root paths. Samples for specialized models use explicit placeholders instead of implying deployed model availability. Video uses the gateway's `/v1/video/generations` and platform task ID; the Demo's `/v1/videos` path is not substituted for this implemented route. Speech writes binary output to a file. Upload examples require local files. Billing examples are explicitly illustrative and do not substitute for live prices or settlement details.

The model-list endpoint lists the selected group's deployed models, without filtering the key's authorization range. Documentation requires both deployment and key authorization. Smart access is independent, candidates stay within authorization, and `myapis-auto` / the PRD alias `veyra-auto` are explained. An unlimited key does not fund its account. Key reveal requires the existing verification flow; the obsolete “shown only once” FAQ was corrected.

Article links support refresh and browser back/forward. Search includes the new translated content. Code and endpoint fields remain LTR in Arabic; navigation, prose and actions follow RTL. Narrow screens use a collapsed grouped directory and local code/table scrolling without page overflow.

## Demo-based layout refinement

At the user's request, the supplied `MyAPIs-Demo.html` was rendered and inspected at desktop and phone widths. The updated documentation adopts its grouped navigation, restrained article layout, page outline and tabbed examples. All 21 PRD articles and their technical content remain available.

- The directory groups articles into getting started, API reference, multimodal APIs, usage/policies, coding tools and help. The phone directory is collapsed by default; choosing an article closes it and returns focus to the article heading. Escape closes the directory and returns focus to its trigger.
- Desktop pages include an in-page outline and previous/next navigation. Quickstart has four sections: account/key preparation, connection configuration, first call, and response/usage checks. Registration, zero-quota limitations and key authorization remain explicit.
- Endpoint editing sits in Quickstart's connection section. Other articles expose compact expandable connection/prerequisite controls. Extended model explanations, environment setup and illustrative responses are expandable rather than competing with the main reading flow.
- cURL, Python and PowerShell use accessible keyboard-operable tabs. Images, audio and video use tabs named for their operation rather than numbered, stacked examples. Protocol tables have tighter spacing. Code, paths and endpoint fields retain LTR direction in Arabic.
- A stable shared code-block component preserves outline anchors when endpoint settings change. Clipboard text and the actual request templates are unchanged. The Demo's simulated funding description was not adopted as production behavior.

Refinement evidence is in `output/docs-redesign/`: the reference screenshots, final production build, formatting, 3,684 translation-key validation and 1,009 passing unit tests (four existing skips). The final 12 browser combinations cover all 21 articles, mobile open/close and Escape, page outline navigation after endpoint edits, keyboard client selection, every multimodal operation's copy action, no page overflow, and successful execution of copied cURL/PowerShell/Python against local fixtures. The existing M9 suite passed nine checks; only four documentation baselines changed, and normal baseline comparison passed. Visually inspected the desktop/phone quickstart, compact protocol table, audio tabs and Arabic dark layouts. The existing service on 5173 serves the new layout. No backend logic, live data, payment connection, commit or push was changed by this presentation refinement.

## Independent Codex guide

Codex now has its own `/docs?article=codex` article and entry under coding tools beside Claude Code, Gemini CLI and Cursor, bringing the directory to 22 articles. The existing configuration example, configurable gateway endpoint, capability boundaries and official configuration link were moved out of the Responses API article. Reciprocal links connect the two guides. Chinese, English and Arabic titles and search indexing now expose Codex directly, including on mobile.

Evidence in `output/docs-codex/` includes a passing production build, formatting and 3,686-key translation check; all 12 documentation browser combinations passed across 22 articles. Checks cover Codex navigation/group placement, direct links and refresh, reciprocal links, search and copied configuration. Existing copied cURL/PowerShell/Python fixtures still pass. M9 passed nine checks with normal screenshot comparison; no new baseline update was needed. Live port 5173 desktop/mobile verification and screenshots confirm the entry is available. This separation does not establish real Codex client compatibility or change gateway behavior.

## Demo address alignment

The default base now matches the Demo constants `https://api.myapis.ai/v1` and `https://api.myapis.ai`. Evidence in `output/docs-demo-address/` includes a passing production build, formatting, translation validation and all 12 documentation browser combinations across 22 articles. Fourteen live checks on port 5173 verify default addresses in Quickstart, Codex, Claude Code, Gemini CLI, Gemini native, Cursor and video, on Chinese desktop and Arabic dark mobile. Custom gateway overrides and copied client fixtures still pass. No real external API request was sent by these checks.

## Acceptance boundaries

PRD sources: `M6-客户控制台与公开页面.md`, UI-2.4; `M6-客户控制台与公开页面.feature`, UI-AC-13. UI-AC-13 is explicitly a **manual** scenario: three people who have never used the platform must start at registration and independently complete a successful cURL or official SDK call within ten minutes, using only Quickstart and without help.

The user confirmed on 2026-11-09 that real payment is not integrated and requested documentation and automation first. New accounts retain zero quota; no free trial credits, funding bypass or simulated payment-to-quota conversion was added. The public guide tells zero-quota readers to pause until real payment and API funding are available. Simulated wallet balance is not callable API quota.

| Requirement | Evidence / state |
| --- | --- |
| UI-2.4 directory and copyable examples | Implemented across all 22 articles |
| UI-2.4 English, Chinese, Arabic | Implemented; catalog completeness and browser matrix checked |
| UI-2.4 product writing and engineering review | Technical contract checks recorded below; product-owner sign-off remains pending |
| UI-AC-13 automated rehearsal | Registration/key/model/quota/call contract plus copied client examples; isolated fixtures only |
| UI-AC-13 three new users, under ten minutes, no assistance | **Pending** real payment/API funding and independent human sessions |
| Arabic native-language review | **Pending** the previously requested native reviewer; automation is not editorial sign-off |

## Reproducible checks

- `go test ./e2e -run '^TestUIAC13QuickstartContract$' -count=1 -v`: uses the real router, isolated SQLite database and local upstream. Registers a user through HTTP, creates an authorized key, discovers models, proves zero-quota denial, explicitly funds only the test account, sends the shared `quickstart-request.json`, verifies text and usage settlement, and rejects an unauthorized model. Fixture funding is not evidence of working payment.
- `npm run test:unit`, `npm run check:i18n`, `npm run build` in `web`: regression, all three catalogs and production compilation. Endpoint validation and shell escaping are covered by focused tests.
- `DOCS_PYTHON=<isolated Python with tests/docs-sdk-requirements.txt installed> node tests/docs-browser.mjs` in `web`: 12 language/theme/viewport combinations, all 21 articles, code direction, no page overflow, deep links/back navigation, search/empty states, endpoint validation, clipboard and keyboard focus. Copied Bash/cURL and Windows PowerShell (on Windows), plus the official Python SDK sample, run against a local HTTP fixture which verifies exact headers/body. No real upstream or payment requests are sent.
- `node tests/m9-browser.mjs`: existing localization regression. Only the four documentation baselines change. For a reviewed documentation-only update, use `M9_UPDATE_SCREENSHOTS=1 M9_SCREENSHOT_FILTER=docs-ar-`; all other screenshots still compare normally.
- CI runs the new Go test with the existing full suite and adds documentation browser/client checks after browser installation.

Initial UI-2.4 execution logs and screenshots are stored under `output/ui24/`; M9 results remain under `output/m9-browser/`. The later layout refinement uses `output/docs-redesign/`. The SDK fixture uses OpenAI Python 3.46.0. Provider tooling may change; transport checks do not certify every feature of Claude Code, Codex, Cursor or Gemini CLI.

### Verified on 2026-11-09

- Go build, vet and the full test suite passed; the new first-call contract passed independently. See `go-build.txt`, `go-vet.txt`, `go-tests.txt` and `go-acceptance.txt` in `output/ui24/`.
- Frontend: 85 unit-test files, 1,009 passes and four existing skips; final production build, changed-source formatting and all 3,653 translation keys passed. See `unit.txt`, `build-final.txt`, `format-final.txt` and `i18n-final.txt`.
- Documentation browser matrix: all 12 combinations and 21 articles per combination passed, with zero page errors. The three copied client examples executed successfully against local fixtures. See `browser/report.json` and `browser-run.txt`. Set `DOCS_BASH` when Git Bash is installed outside the default Windows path.
- Existing M9 suite: nine acceptance checks passed with zero page errors in the final normal comparison run. All four updated documentation screenshots had zero changed pixels. Only those four baselines were updated. The earlier run's page-load timeout did not reproduce after source edits stopped; retain the earlier log separately from the final successful `m9-compare-final.txt`.
- Visually inspected Chinese/Arabic desktop and mobile pages, light/dark themes and the LTR PowerShell code inside RTL prose. The running local Vite service on 5173 serves the new documentation source (`live-source.json`). No backend restart, live business-data mutation, real payment or external model request was needed.

## Human acceptance sheet

Prerequisites: real self-service payment and callable quota posting; a deployed, priced Chat Completions model; registration and key creation working in the target environment; frozen document revision. Test users start with new zero-quota accounts. Do not pre-credit them or explain the steps. Record payment-provider delays separately without removing them from elapsed time.

| Participant | Prior platform use | Language / device | Start at registration | First successful response | Total <= 10 min | Assistance | Result |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A | Must be none | Pending | Pending | Pending | Pending | Must be none | Not run |
| B | Must be none | Pending | Pending | Pending | Pending | Must be none | Not run |
| C | Must be none | Pending | Pending | Pending | Pending | Must be none | Not run |

Retain the tested document revision, anonymized timestamps, HTTP status, response text and trace ID. Never retain passwords, full API keys or payment details. A block or request for help is a failure to record and fix, not a reason to silently restart the clock. Product-owner, engineer and Arabic reviewer sign-offs are separate from these results.

## Official references checked

- [OpenAI Python SDK](https://github.com/openai/openai-python): SDK initialization with custom base URL and environment credentials.
- [Codex configuration](https://developers.openai.com/codex/config-reference/): custom provider, `env_key`, `base_url` and Responses wire protocol.
- [Gemini CLI configuration](https://geminicli.com/docs/reference/configuration/) and [authentication](https://geminicli.com/docs/get-started/authentication/): API-key mode, gateway root override and API version configuration.
- [Claude Code gateway documentation](https://code.claude.com/docs/en/llm-gateway): gateway configuration and compatibility boundaries.

References were checked on 2026-11-09. Local router/DTO and authorization behavior take precedence over vendor examples for this gateway's supported contract.
