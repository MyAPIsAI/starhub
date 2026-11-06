import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { ServiceLevelValue } from "../components/service-level-value";
import {
  useServiceGroups,
  useUpdateServiceGroupSLA,
  type ServiceGroup,
} from "../lib/api/service-group";
import {
  parseServiceLevelTargets,
  readServiceLevel,
  successPercent,
} from "../lib/service-level";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./service-group-targets.css";

type Draft = {
  group: ServiceGroup;
  revision: number;
  percent: string;
  latency: string;
  remove: boolean;
};

export function ServiceGroupTargets() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const allowed = role === ROLE_ADMIN || role === ROLE_ROOT;
  const query = useServiceGroups(allowed);
  const mutation = useUpdateServiceGroupSLA();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  if (!allowed) return null;
  const data = query.isError ? undefined : query.data;
  const rows =
    data?.filter((g) =>
      `${g.group_id} ${g.name}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    ) ?? [];
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const targets = draft
    ? parseServiceLevelTargets(draft.percent, draft.latency)
    : undefined;
  const current =
    draft && data?.find((g) => g.group_id === draft.group.group_id);
  const conflict =
    !!draft &&
    (!current ||
      current.id !== draft.group.id ||
      readServiceLevel(current.sla)?.revision !== draft.revision);
  const begin = (group: ServiceGroup, remove = false) => {
    const sla = readServiceLevel(group.sla);
    if (!sla || !Number.isSafeInteger(group.id) || group.id <= 0) return;
    mutation.reset();
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      group,
      revision: sla.revision,
      remove,
      percent:
        sla.targets?.success_rate_ppm == null
          ? ""
          : successPercent(sla.targets.success_rate_ppm),
      latency:
        sla.targets?.p95_latency_ms == null
          ? ""
          : String(sla.targets.p95_latency_ms),
    });
  };
  const close = () => {
    if (!mutation.isPending) {
      setDraft(null);
      setError("");
    }
  };
  const save = async () => {
    if (!draft || !review || conflict || !data || (!draft.remove && !targets))
      return;
    try {
      const result = await mutation.mutateAsync({
        group: draft.group.group_id,
        expected_group_id: draft.group.id,
        expected_revision: draft.revision,
        targets: draft.remove ? null : targets!,
      });
      const saved = readServiceLevel(result.sla);
      if (
        !saved ||
        result.id !== draft.group.id ||
        saved.revision !== draft.revision + 1
      )
        throw new Error("Invalid target update acknowledgement");
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage("targets_saved_v_08bef9a9bb", {
            p0: result.group_id,
            p1: saved.revision,
          }),
        ),
      );
    } catch {
      setError(
        text(
          ...translatedMessage(
            "save_outcome_is_unconfirmed_refresh_to_check__87e91df437",
          ),
        ),
      );
    }
  };
  return (
    <section
      className="service-group-targets"
      aria-labelledby="group-target-heading"
    >
      <header>
        <div>
          <h2 id="group-target-heading">
            {text(...translatedMessage("group_sla_targets_0a5619344f"))}
          </h2>
          <p>
            {text(
              ...translatedMessage(
                "only_database_groups_are_listed_operator_publ_7c1a94ca78",
              ),
            )}
          </p>
        </div>
        <Button
          disabled={query.isFetching}
          onClick={() => {
            setNotice("");
            void query.refetch();
          }}
        >
          {text(...translatedMessage("refresh_targets_f3294d8db7"))}
        </Button>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_targets_8f807ce4e6"))}
        </p>
      )}
      {query.isError && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "cannot_load_group_targets_cached_data_is_hidd_049472cfe0",
            ),
          )}
        </p>
      )}
      {notice && (
        <p role="status" className="sgt-notice">
          {notice}
        </p>
      )}
      <label>
        {text(...translatedMessage("search_target_groups_b9b5508b01"))}
        <input
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setPage(0);
          }}
        />
      </label>
      {data?.length === 0 && (
        <p>
          {text(
            ...translatedMessage(
              "no_database_groups_create_and_verify_a_real_g_7c3ae7d5f1",
            ),
          )}
        </p>
      )}
      <div
        className="sgt-table"
        tabIndex={0}
        role="region"
        aria-label={text(...translatedMessage("group_target_list_401779db95"))}
      >
        <table>
          <thead>
            <tr>
              <th>{text(...translatedMessage("group_528d77dde5"))}</th>
              <th>
                {text(...translatedMessage("published_targets_f0e57608c9"))}
              </th>
              <th>{text(...translatedMessage("revision_2d6025d8da"))}</th>
              <th>{text(...translatedMessage("actions_d83f4ed14a"))}</th>
            </tr>
          </thead>
          <tbody>
            {rows.slice(currentPage * 20, currentPage * 20 + 20).map((g) => {
              const sla = readServiceLevel(g.sla);
              const editable = !!sla && Number.isSafeInteger(g.id) && g.id > 0;
              return (
                <tr key={g.id} data-target-group={g.group_id}>
                  <td>
                    <strong>{g.name}</strong>
                    <div>
                      <bdi>{g.group_id}</bdi> ·{" "}
                      {g.enabled
                        ? text(...translatedMessage("enabled_f028916a96"))
                        : text(...translatedMessage("disabled_0049e8e608"))}
                    </div>
                  </td>
                  <td>
                    <ServiceLevelValue value={g.sla} text={text} />
                  </td>
                  <td>
                    <bdi>{sla ? `v${sla.revision}` : "—"}</bdi>
                  </td>
                  <td>
                    <div className="sgt-actions">
                      <Button disabled={!editable} onClick={() => begin(g)}>
                        {text(...translatedMessage("edit_targets_8f3f498a50"))}
                      </Button>
                      <Button
                        disabled={!editable || !sla?.targets}
                        onClick={() => begin(g, true)}
                      >
                        {text(...translatedMessage("unpublish_adacb36b31"))}
                      </Button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {data && (
        <div className="sgt-actions">
          <Button
            disabled={currentPage === 0}
            onClick={() => setPage(currentPage - 1)}
          >
            {text(...translatedMessage("previous_2672646f39"))}
          </Button>
          <span>
            {currentPage + 1} / {pages}
          </span>
          <Button
            disabled={currentPage >= pages - 1}
            onClick={() => setPage(currentPage + 1)}
          >
            {text(...translatedMessage("next_c0fd39244f"))}
          </Button>
        </div>
      )}
      <Modal
        open={!!draft}
        onOpenChange={(open) => !open && close()}
        title={
          draft?.remove
            ? text(...translatedMessage("unpublish_targets_914bb56980"))
            : text(...translatedMessage("edit_group_targets_d965e16a06"))
        }
        description={text(
          ...translatedMessage(
            "saving_requires_the_same_group_identity_and_t_9853f52d9f",
          ),
        )}
        footer={
          <>
            <Button disabled={mutation.isPending} onClick={close}>
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            {review ? (
              <Button
                variant="primary"
                disabled={
                  mutation.isPending ||
                  conflict ||
                  !data ||
                  (!draft?.remove && !targets)
                }
                onClick={() => void save()}
              >
                {text(...translatedMessage("confirm_targets_1d25b8f610"))}
              </Button>
            ) : (
              <Button
                variant="primary"
                disabled={!targets || conflict || !data}
                onClick={() => setReview(true)}
              >
                {text(...translatedMessage("review_targets_03a65a5ae3"))}
              </Button>
            )}
          </>
        }
      >
        {draft && (
          <>
            <p>
              <strong>{draft.group.name}</strong> ·{" "}
              <bdi>{draft.group.group_id}</bdi> · v{draft.revision}
            </p>
            {review ? (
              <div>
                <p>
                  {draft.remove
                    ? text(
                        ...translatedMessage(
                          "confirming_clears_this_group_s_published_targ_4ede2e4a81",
                        ),
                      )
                    : text(
                        ...translatedMessage("targets_to_publish_f754e6971c"),
                      )}
                </p>
                {!draft.remove && (
                  <ServiceLevelValue
                    value={{ revision: 1, updated_at: 1, targets }}
                    text={text}
                  />
                )}
                {!draft.remove && (
                  <Button
                    onClick={() => setReview(false)}
                    disabled={mutation.isPending}
                  >
                    {text(...translatedMessage("back_to_edit_8e464c075a"))}
                  </Button>
                )}
              </div>
            ) : (
              <div className="sgt-fields">
                <label>
                  {text(...translatedMessage("success_target_2b8f8e1a39"))}
                  <input
                    dir="ltr"
                    inputMode="decimal"
                    value={draft.percent}
                    onChange={(e) =>
                      setDraft({ ...draft, percent: e.target.value })
                    }
                  />
                </label>
                <label>
                  {text(...translatedMessage("p95_target_ms_858cec53dc"))}
                  <input
                    dir="ltr"
                    inputMode="numeric"
                    value={draft.latency}
                    onChange={(e) =>
                      setDraft({ ...draft, latency: e.target.value })
                    }
                  />
                </label>
                <p>
                  {text(
                    ...translatedMessage(
                      "success_0_100_up_to_four_decimal_places_p95_1_9cb8040148",
                    ),
                  )}
                </p>
              </div>
            )}
            {conflict && (
              <p role="alert">
                {text(
                  ...translatedMessage(
                    "the_group_or_revision_changed_or_the_current__2ec8cb9f41",
                  ),
                )}
              </p>
            )}
            {error && <p role="alert">{error}</p>}
          </>
        )}
      </Modal>
    </section>
  );
}
