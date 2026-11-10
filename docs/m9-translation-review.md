# M9 translation review record

Status: **Pending native Arabic review and product terminology approval.** The user authorized implementation and automated tests first because no reviewer is currently available. This record is not approval of the text.

English is the source language. Review both `web/src/i18n/{en,zh,ar}.json` and `web/src/i18n/messages.{en,zh,ar}.json`, plus separately managed legal documents, announcements and mail templates when they change. Technical identifiers and user-authored records retain their original spelling.

| Required evidence | Current status |
| --- | --- |
| Reviewed catalog commit or SHA-256 and changed-key list | Record against the exact revision submitted for review |
| Product owner and approved terminology changes | Pending; draft glossary is in `m9-localization.md` |
| Chinese review, reviewer and date | Pending editorial sign-off |
| Arabic native reviewer and date | Pending reviewer assignment |
| Corrections, recheck and final approval | Pending review |
| Local automated verification | See the dated verification evidence in `m9-localization.md` and `web/HANDOFF.md` |
| Remote CI run for the submitted commit | Record its URL after observing a completed run |

Review scope includes navigation, forms, validation messages, accessibility labels, financial terminology, documentation and RTL screenshots. Confirm interpolation order and that model names, keys, URLs, trace IDs and currency amounts remain readable. Every later source or translation change invalidates approval for the affected keys until reviewed again. Keep draft status until the actual reviewer and product owner provide their evidence.
