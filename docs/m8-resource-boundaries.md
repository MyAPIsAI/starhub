# Resource synchronization permission boundaries

This implementation covers M8 RES-5.3 and RES-5.5, with RES-AC-21 and RES-AC-24. The staff page is **Channels and placement** at `/admin?tab=resources`; Technical and Root can manage inventory. Source synchronization never grants a mapping, group placement or gateway route by itself.

## Intake, review and removal

- New source instance IDs start with `modelId: null`, `groupIds: []`, `enabled: false`. Renaming an external ID is a removal and new intake because the demo contract supplies no independent stable identity. No identity equivalence or authorization is inferred.
- Changing an existing instance's upstream model, canonical model, channel ID or channel provider pauses its placement and revokes affected managed gateway routes. The original and latest identities are persisted in `placement_reviews`. Repeated synchronization, reverting the source identity and restarting the backend do not clear pending review.
- The editor shows the changed identity and requires an explicit checkbox before restoring an enabled placement. `PUT /api/placement/{resource_id}` requires the current `expected_revision` and `placement.confirmIdentity: true` when an identity review is pending. Saving a disabled placement retains the review. Enabling also retains the existing requirements for a mapped model, groups, an enabled source channel and an available resource.
- Confirmation saves the local placement. A separate gateway preview and publication is still required to resume managed calls. A gateway owned by a different source must first be explicitly released through the existing binding workflow; source synchronization cannot change ownership.
- Removed source instances immediately stop participating in managed calls. Their previous mapping, groups, source identity, removal time and source sequence are stored separately in `resource_placement_archives`, outside all routing queries. If the same ID reappears, it starts unmapped and unplaced; its earlier archive remains available for inspection.
- The page displays the latest 100 archived entries and the total count. Reviewed entries can be cleared individually with a two-step confirmation. Clearing the current page exposes older entries. `DELETE /api/resource/removed-placements/{archive_id}` requires `{"expected_revision":N,"confirm":true}`. Technical and Root have this specific archive permission; other roles do not. Stale revisions return 409. Cleanup does not remove live resources, active placements, call logs or gateway publication history. The existing audit middleware records the attempted operation.

## Demo JSON import contract

`PUT /api/resource` accepts `{"expected_revision":N,"snapshot":{...}}`. A snapshot contains `id`, positive increasing `sequence`, RFC3339 `publishedAt`, optional `source`/`receivedAt`, `channels` and `instances`. The backend owns receipt time. Channel entries require `id`, `name` and boolean `enabled`; optional `provider` identifies the supplier. Instances require `id`, `channelId`, `upstreamModel`, `canonicalModel`, `protocols`, `capabilities`, `contextLength`, `status` and `latencyMs`; `region` and supported observation fields are optional. The current frontend's import template includes the optional strings as empty strings.

The full catalog response also contains local `placements`, regions, revision, pending reviews and archived mappings. These local fields are not source-owned snapshot inputs: importing source-supplied placements is rejected rather than allowing synchronization to widen authorization.

Validation checks increasing sequence, unique channel/resource IDs, valid channel references, supported protocol/capability/status values, bounded finite numeric values, required field presence and correct JSON types. Explicit `null`, unknown fields, fractional integer fields and malformed timestamps are rejected. In particular, `enabled: null` cannot silently turn into `false`, and missing numeric fields cannot silently turn into zero. Explicit valid zero values remain supported.

Validation and application share the existing transaction boundary: failure leaves the published snapshot, revision, local placements, pending reviews, archives, gateway ownership and routes unchanged. An archive-write failure also rolls back route invalidation and snapshot replacement.

This remains the **demo snapshot contract**, not certification of the real resource platform's incremental delivery protocol. The user confirmed the real interface is undecided. Before production integration, agree on stable identity, field definitions and authentication with that platform.

## Verification

- `e2e/resource_boundaries_acceptance_test.go`: RES-AC-21 uses a real isolated HTTP relay and upstream counter, checking new intake denial, successful mapped calls, upstream/channel/provider changes, explicit confirmation, release/republication, external ID replacement, no charge for denied calls, cleanup confirmation/version control and audit.
- `e2e/resource_snapshot_validation_test.go`: RES-AC-24 starts with a published route, rejects nonincreasing sequence, duplicate IDs, bad references, invalid enums/types/dates/numbers and injected placements, and compares the complete catalog and binding state after each rejection.
- `model/resource_placement_review_test.go`: review and archive persistence across database restart, identity rollback requiring review, reintroduced IDs, targeted cleanup and injected transactional rollback.
- Frontend catalog tests cover strict imports and removal/reintroduction without authorization inheritance. `web/tests/resource-boundaries-browser.mjs` uses isolated HTTP fixtures to verify staff entry points, confirmation, keyboard return, version conflict handling and invalid import rejection across Chinese/English, light/dark, desktop/mobile; Arabic staff preference uses the existing English fallback. These UI fixtures do not write to the live database.

Current local evidence is recorded under `output/res-boundaries-*`; see [the handoff](../web/HANDOFF.md) for final results and activation status.
