import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useResourceCatalog, useRegionMutation } from "../lib/api/resource";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../lib/system-access";
import { useSettingsText } from "./settings-text";
import "./resource-regions.css";

type Draft = {
  revision: number;
  id: string;
  name: string;
  description: string;
  locked: boolean;
  remove: boolean;
};

export function ResourceRegions() {
  const { text } = useSettingsText();
  const role = useSystemAccess((s) => s.role);
  const query = useResourceCatalog();
  const mutation = useRegionMutation();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [review, setReview] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const { hash } = useLocation();
  const section = useRef<HTMLElement>(null);
  useEffect(() => {
    if (hash !== "#regions" || (role !== ROLE_ADMIN && role !== ROLE_ROOT))
      return;
    const frame = requestAnimationFrame(() => {
      section.current?.scrollIntoView({ block: "start" });
      section.current?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [hash, role, query.isLoading]);
  if (role !== ROLE_ADMIN && role !== ROLE_ROOT) return null;
  const data = query.error ? undefined : query.data;
  const counts = new Map<string, number>();
  for (const resource of data?.snapshot?.instances ?? [])
    counts.set(resource.region, (counts.get(resource.region) ?? 0) + 1);
  const rows = [
    ...new Set([...Object.keys(data?.regions ?? {}), ...counts.keys()]),
  ]
    .sort()
    .filter((id) =>
      `${id} ${data?.regions[id]?.name ?? ""}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const pages = Math.max(1, Math.ceil(rows.length / 20));
  const currentPage = Math.min(page, pages - 1);
  const conflict = Boolean(draft && data && draft.revision !== data.revision);
  const begin = (id: string, remove = false) => {
    if (!data) return;
    const region = data.regions[id];
    mutation.reset();
    setError("");
    setNotice("");
    setReview(remove);
    setDraft({
      revision: data.revision,
      id,
      name: region?.name ?? "",
      description: region?.description ?? "",
      locked: Boolean(id),
      remove,
    });
  };
  const close = () => {
    if (!mutation.isPending) {
      setDraft(null);
      mutation.reset();
      setError("");
    }
  };
  const save = async () => {
    if (!draft || !data || conflict || !review) return;
    try {
      const result = await mutation.mutateAsync(
        draft.remove
          ? {
              operation: "delete",
              id: draft.id,
              expected_revision: draft.revision,
            }
          : {
              operation: "save",
              id: draft.id.trim(),
              expected_revision: draft.revision,
              name: draft.name.trim(),
              description: draft.description.trim(),
            },
      );
      setDraft(null);
      setNotice(
        text(
          ...translatedMessage("region_directory_saved_at_v_e0b35096e7", {
            p0: result.revision,
          }),
        ),
      );
    } catch {
      setError(
        text(
          ...translatedMessage(
            "save_outcome_is_unconfirmed_refresh_to_check__a2d3451c98",
          ),
        ),
      );
    }
  };
  return (
    <section
      className="resource-regions"
      id="regions"
      ref={section}
      tabIndex={-1}
      aria-labelledby="region-heading"
    >
      <header>
        <div>
          <h2 id="region-heading">
            {text(...translatedMessage("resource_region_directory_7715c4edf7"))}
          </h2>
          <p>
            {text(
              ...translatedMessage(
                "manage_region_names_and_descriptions_referenc_a4673b08ee",
              ),
            )}
          </p>
        </div>
        <div className="rr-actions">
          <Button
            disabled={query.isFetching}
            onClick={() => {
              setNotice("");
              void query.refetch();
            }}
          >
            {text(...translatedMessage("refresh_regions_c355affe48"))}
          </Button>
          <Button disabled={!data} onClick={() => begin("")}>
            {text(...translatedMessage("add_region_e053b240d1"))}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(...translatedMessage("loading_regions_32aec83706"))}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "region_directory_is_unavailable_refresh_to_re_9f28463e76",
            ),
          )}
        </p>
      )}
      {notice && (
        <p role="status" className="rr-notice">
          {notice}
        </p>
      )}
      {data && (
        <>
          <label className="rr-search">
            {text(...translatedMessage("search_regions_1b669aeae0"))}
            <input
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
            />
          </label>
          <p>
            v{data.revision} ·{" "}
            {text(
              ...translatedMessage(
                "definitions_and_source_observations_stay_sepa_0c4059070a",
              ),
            )}
          </p>
          {!rows.length ? (
            <p>{text(...translatedMessage("no_regions_yet_b023745ccf"))}</p>
          ) : (
            <>
              <div
                className="rr-table"
                role="region"
                tabIndex={0}
                aria-label={text(
                  ...translatedMessage("region_directory_table_e548446267"),
                )}
              >
                <table>
                  <thead>
                    <tr>
                      {[
                        text(...translatedMessage("region_id_name_6ba135fc8a")),
                        text(...translatedMessage("description_c6bae5d4e7")),
                        text(
                          ...translatedMessage(
                            "inventory_references_0d4785f103",
                          ),
                        ),
                        text(...translatedMessage("actions_d83f4ed14a")),
                      ].map((label) => (
                        <th key={label}>{label}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rows
                      .slice(currentPage * 20, currentPage * 20 + 20)
                      .map((id) => {
                        const region = data.regions[id];
                        const count = counts.get(id) ?? 0;
                        return (
                          <tr key={id} data-region-id={id}>
                            <td>
                              <strong>
                                {region?.name ??
                                  (id
                                    ? text(
                                        ...translatedMessage(
                                          "unregistered_region_2610c9c2bd",
                                        ),
                                      )
                                    : text(
                                        ...translatedMessage(
                                          "unknown_region_4215977522",
                                        ),
                                      ))}
                              </strong>
                              <code>{id || "—"}</code>
                            </td>
                            <td>{region?.description || "—"}</td>
                            <td>{count}</td>
                            <td>
                              {id && (
                                <div className="rr-actions">
                                  <Button onClick={() => begin(id)}>
                                    {region
                                      ? text(
                                          ...translatedMessage(
                                            "edit_region_ded14e3350",
                                          ),
                                        )
                                      : text(
                                          ...translatedMessage(
                                            "register_region_69dfd204ef",
                                          ),
                                        )}
                                  </Button>
                                  {region && (
                                    <Button
                                      disabled={count > 0}
                                      title={
                                        count > 0
                                          ? text(
                                              ...translatedMessage(
                                                "current_inventory_references_this_region_af90414c3e",
                                              ),
                                            )
                                          : undefined
                                      }
                                      onClick={() => begin(id, true)}
                                    >
                                      {text(
                                        ...translatedMessage(
                                          "delete_region_2be7f9d133",
                                        ),
                                      )}
                                    </Button>
                                  )}
                                </div>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                  </tbody>
                </table>
              </div>
              <nav
                className="rr-pagination"
                aria-label={text(
                  ...translatedMessage("region_pagination_0d3729039e"),
                )}
              >
                <Button
                  disabled={currentPage === 0}
                  onClick={() => setPage(currentPage - 1)}
                >
                  {text(...translatedMessage("previous_2672646f39"))}
                </Button>
                <span>
                  {currentPage + 1} / {pages} · {rows.length}
                </span>
                <Button
                  disabled={currentPage >= pages - 1}
                  onClick={() => setPage(currentPage + 1)}
                >
                  {text(...translatedMessage("next_c0fd39244f"))}
                </Button>
              </nav>
            </>
          )}
        </>
      )}
      <Modal
        open={Boolean(draft)}
        onOpenChange={(open) => !open && close()}
        preventOutsideClose
        title={
          draft?.remove
            ? text(...translatedMessage("confirm_region_deletion_6472d3fca7"))
            : review
              ? text(...translatedMessage("confirm_region_details_c0d02f47c0"))
              : text(...translatedMessage("edit_region_definition_0c5622f111"))
        }
        description={text(
          ...translatedMessage(
            "only_the_inventory_region_directory_changes_g_cc3d0a8f24",
          ),
        )}
        footer={
          <>
            <Button disabled={mutation.isPending} onClick={close}>
              {text(...translatedMessage("cancel_0828e4ce08"))}
            </Button>
            {review && !draft?.remove && (
              <Button
                disabled={mutation.isPending}
                onClick={() => setReview(false)}
              >
                {text(...translatedMessage("back_to_editing_85b057f793"))}
              </Button>
            )}
            <Button
              disabled={!data || conflict || mutation.isPending}
              onClick={() => {
                if (review) {
                  void save();
                  return;
                }
                if (!draft?.id.trim() || !draft.name.trim()) {
                  setError(
                    text(
                      ...translatedMessage(
                        "enter_a_region_identifier_and_name_d5b528178e",
                      ),
                    ),
                  );
                  return;
                }
                setError("");
                setReview(true);
              }}
            >
              {mutation.isPending
                ? text(...translatedMessage("saving_b89854a86e"))
                : review
                  ? draft?.remove
                    ? text(...translatedMessage("confirm_delete_f932f7d779"))
                    : text(...translatedMessage("confirm_save_f90ef9ab0e"))
                  : text(...translatedMessage("review_changes_0594a17c6d"))}
            </Button>
          </>
        }
      >
        <div className="rr-editor">
          {conflict && (
            <p role="alert">
              {text(
                ...translatedMessage(
                  "the_directory_revision_changed_cancel_and_reo_82223e72e0",
                ),
              )}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {draft &&
            (review ? (
              <>
                <p>
                  <strong>{draft.name}</strong> · <code>{draft.id}</code> · v
                  {draft.revision}
                </p>
                <p>{draft.description || "—"}</p>
                <p>
                  {text(
                    ...translatedMessage(
                      "referenced_by_resources_in_the_current_snapsh_5915ac569a",
                      { p0: counts.get(draft.id) ?? 0 },
                    ),
                  )}
                </p>
              </>
            ) : (
              <>
                <label>
                  {text(...translatedMessage("region_identifier_c7634ae267"))}
                  <input
                    value={draft.id}
                    readOnly={draft.locked}
                    onChange={(e) => setDraft({ ...draft, id: e.target.value })}
                  />
                </label>
                <label>
                  {text(...translatedMessage("display_name_5900b00dfb"))}
                  <input
                    value={draft.name}
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </label>
                <label>
                  {text(...translatedMessage("region_description_29bf5b17c6"))}
                  <textarea
                    rows={3}
                    value={draft.description}
                    onChange={(e) =>
                      setDraft({ ...draft, description: e.target.value })
                    }
                  />
                </label>
              </>
            ))}
        </div>
      </Modal>
    </section>
  );
}
