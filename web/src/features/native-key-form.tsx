import { message as translatedMessage } from "../i18n/messages";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { ChevronDown } from "lucide-react";
import { Drawer } from "../components/ui/overlay";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { useTokenChoices } from "../lib/api/token-choices";
import {
  useCreateToken,
  useUpdateToken,
  readOwnedKeyReceipt,
  type Token,
} from "../lib/api/tokens";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useSessionQueryIdentity } from "../lib/system-access";
import type { PreviewKey } from "../lib/product-preview";
import { isSmartModelId } from "../lib/smart-model";
import {
  keySpendingLimitValue,
  remainingQuotaForLifetimeLimit,
} from "../lib/key-spending-limit";
import { copyText } from "./code-example";
import {
  NativeKeyModelPolicy,
  type NativeModelScope,
  type NativePolicyError,
} from "./native-key-model-policy";
import "./native-key-form.css";
export function NativeKeyForm({
  item,
  onClose,
  onCreated,
  initialModel,
  initialGroup,
}: {
  item?: PreviewKey;
  onClose: () => void;
  onCreated?: (token: Token) => void;
  initialModel?: string;
  initialGroup?: string;
}) {
  const { i18n } = useTranslation(),
    navigate = useNavigate(),
    scope = useSessionQueryIdentity(),
    query = useTokenChoices(),
    create = useCreateToken(),
    update = useUpdateToken(),
    { keys } = useBackendKeys();
  const c = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const choices = query.isError ? undefined : query.data,
    initialRevision = useRef<string>();
  const choicesFailed = query.isError && !query.isFetching;
  if (!initialRevision.current && choices && !query.isFetching)
    initialRevision.current = choices.revision;
  const [name, setName] = useState(item?.name ?? ""),
    [group, setGroup] = useState(item?.tier ?? initialGroup ?? ""),
    [models, setModels] = useState<string[] | null>(
      item?.allowedModels ?? null,
    ),
    [quota, setQuota] = useState(() => keySpendingLimitValue(item)),
    [ips, setIps] = useState(item?.ipWhitelist?.join("\n") ?? ""),
    [error, setError] = useState(""),
    [operation, setOperation] = useState(""),
    [receipt, setReceipt] = useState(""),
    [created, setCreated] = useState<Token | null>(null),
    [busy, setBusy] = useState(false);
  const [smartEnabled, setSmartEnabled] = useState(item?.smartAccess ?? true);
  const [period, setPeriod] = useState(item?.quotaPeriod ?? "never");
  const [expiry, setExpiry] = useState(() =>
    item?.validUntil
      ? new Date(
          new Date(item.validUntil).getTime() -
            new Date(item.validUntil).getTimezoneOffset() * 60000,
        )
          .toISOString()
          .slice(0, 16)
      : "",
  );
  const [smartDirty, setSmartDirty] = useState(false);
  const [expiryMode, setExpiryMode] = useState<"never" | "date">(
    item?.validUntil ? "date" : "never",
  );
  const [accessOpen, setAccessOpen] = useState(false);
  const [accessError, setAccessError] = useState<"expiry" | "ips" | null>(null);
  const expiryInput = useRef<HTMLInputElement>(null);
  const ipInput = useRef<HTMLTextAreaElement>(null);
  const ipEntries = ips
    .split(/\r?\n/)
    .map((value) => value.trim())
    .filter(Boolean);
  const [candidateModels, setCandidateModels] = useState<string[] | null>(
    item?.smartCandidates ?? null,
  );
  const pending = useRef(false);
  const [modelScope, setModelScope] = useState<NativeModelScope>(
    item ? "custom" : "snapshot",
  );
  const [snapshotModels, setSnapshotModels] = useState<string[] | null>(null);
  const [policyDirty, setPolicyDirty] = useState(false);
  const [policyError, setPolicyError] = useState<NativePolicyError>(null);
  const [validationAttempt, setValidationAttempt] = useState(0);
  const groups = [...new Set(choices?.choices.map((c) => c.group) ?? [])];
  const effectiveGroup = groups.includes(group)
    ? group
    : !item
      ? (groups.find((g) =>
          choices?.choices.some(
            (c) => c.group === g && c.model === initialModel,
          ),
        ) ??
        groups[0] ??
        "")
      : group;
  const available = [
    ...new Set(
      choices?.choices
        .filter((c) => c.group === effectiveGroup)
        .map((c) => c.model) ?? [],
    ),
  ];
  const preservingAutoGroup =
    item?.tier === "auto" && effectiveGroup === "auto";
  const selected = preservingAutoGroup
    ? (item.allowedModels ?? [])
    : modelScope === "snapshot"
      ? (snapshotModels ?? available)
      : (models ?? (item?.modelAccess === "all" ? available : []));
  const policyChanged =
    !item ||
    effectiveGroup !== item.tier ||
    (!preservingAutoGroup &&
      policyDirty &&
      (modelScope === "snapshot" ||
        selected.join(",") !== (item.allowedModels ?? []).join(",")));
  const quotaChanged =
    !item ||
    quota !== keySpendingLimitValue(item) ||
    period !== (item.quotaPeriod ?? "never");
  const smartAvailable = selected.filter(
    (model) =>
      preservingAutoGroup ||
      choices?.choices.some(
        (choice) =>
          choice.group === effectiveGroup &&
          choice.model === model &&
          choice.smart_eligible !== false,
      ),
  );
  const smartSelected = (candidateModels ?? smartAvailable).filter((model) =>
    smartAvailable.includes(model),
  );
  const smartPolicyChanged = !item || smartDirty || policyChanged;
  const reusable = initialModel
    ? keys.find(
        (k) =>
          k.active &&
          choices?.choices.some(
            (c) =>
              c.group === k.tier &&
              (isSmartModelId(initialModel)
                ? k.smartAccess === true
                : c.model === initialModel &&
                  (k.modelAccess === "all" ||
                    k.allowedModels?.includes(initialModel))),
          ),
      )
    : undefined;
  async function submit() {
    if (
      pending.current ||
      operation ||
      !choices ||
      !initialRevision.current ||
      query.isFetching ||
      query.isError
    )
      return;
    if (!name.trim()) {
      setError(c(...translatedMessage("enter_a_key_name_d5f5c413f4")));
      return;
    }
    if (
      policyChanged &&
      (!selected.length ||
        selected.some(
          (m) =>
            !available.includes(m) &&
            !(item?.tier === effectiveGroup && item.allowedModels?.includes(m)),
        ))
    ) {
      setError("");
      setPolicyError("models");
      setValidationAttempt((value) => value + 1);
      return;
    }
    let remain: number | undefined;
    if (smartPolicyChanged && smartEnabled && smartSelected.length === 0) {
      setError("");
      setPolicyError("candidates");
      setValidationAttempt((value) => value + 1);
      return;
    }
    setPolicyError(null);
    if (
      expiryMode === "date" &&
      (!expiry || !Number.isFinite(new Date(expiry).getTime()))
    ) {
      setError("");
      setAccessError("expiry");
      setAccessOpen(true);
      requestAnimationFrame(() => expiryInput.current?.focus());
      return;
    }
    if (ipEntries.length > 50) {
      setError("");
      setAccessError("ips");
      setAccessOpen(true);
      requestAnimationFrame(() => ipInput.current?.focus());
      return;
    }
    setAccessError(null);
    if (quotaChanged && quota !== "") {
      try {
        remain = remainingQuotaForLifetimeLimit(quota);
      } catch (e) {
        const reason = e instanceof Error ? e.message : "invalid_amount";
        setError(
          reason === "below_used"
            ? c(
                ...translatedMessage(
                  "the_lifetime_limit_cannot_be_below_the_amount_3944eda1d7",
                ),
              )
            : reason === "too_large"
              ? c(
                  ...translatedMessage(
                    "spending_limit_is_too_large_0337b3a1f6",
                  ),
                )
              : c(
                  ...translatedMessage(
                    "enter_a_nonnegative_usd_amount_with_at_most_s_c0e1e0b5fa",
                  ),
                ),
        );
        return;
      }
    }
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      const payload = {
        name: name.trim(),
        ...(policyChanged
          ? { group: effectiveGroup, model_limits: selected.join(",") }
          : {}),
        ...(smartPolicyChanged
          ? {
              smart_access: smartEnabled,
              smart_candidates: smartSelected.join(","),
            }
          : {}),
        allow_ips: ipEntries.join(","),
        ...(quotaChanged ? { unlimited_quota: quota === "" } : {}),
        ...(remain === undefined ? {} : { quota_limit: remain }),
        ...(quotaChanged ? { quota_period: period } : {}),
        expired_time:
          expiryMode === "date"
            ? Math.floor(new Date(expiry).getTime() / 1000)
            : -1,
        choices_revision: initialRevision.current,
      };
      if (item) {
        await update.mutateAsync({
          ...payload,
          id: Number(item.id),
          expected_revision: item.managementRevision,
          expected_quota: item.quota,
        });
        onClose();
      } else {
        const token = await create.mutateAsync(payload);
        setCreated(token);
        onCreated?.(token);
      }
    } catch (e) {
      setError(
        e instanceof Error && e.message.startsWith("key limit reached:")
          ? c(
              ...translatedMessage(
                "this_account_has_reached_the_200_key_limit_de_00fc4973c6",
              ),
            )
          : e instanceof Error
            ? e.message
            : String(e),
      );
      if (!(
        e instanceof Object &&
        "status" in e &&
        [400, 404, 409].includes(Number(e.status))
      ))
        setOperation((e as { operationId?: string })?.operationId ?? "");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function checkReceipt() {
    if (!operation || pending.current) return;
    pending.current = true;
    setBusy(true);
    try {
      const result = await readOwnedKeyReceipt(operation, scope);
      setReceipt(
        c(
          ...translatedMessage(
            "operation_confirmed_for_key_reload_keys_plain_fbd58c10fe",
            { p0: result.token_id },
          ),
        ),
      );
    } catch {
      setReceipt(
        c(
          ...translatedMessage(
            "no_confirmed_receipt_was_found_this_does_not__0e45db0dbc",
          ),
        ),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      open
      preventOutsideClose
      onOpenChange={(open) => {
        if (!open && !pending.current) onClose();
      }}
      title={
        created
          ? c(...translatedMessage("key_created_23df770140"))
          : item
            ? c(...translatedMessage("edit_key_cff9d0c9e8"))
            : c(...translatedMessage("create_key_06433c30b0"))
      }
      description={c(
        ...translatedMessage(
          "set_the_service_group_model_permissions_spend_7bedf4c9cf",
        ),
      )}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {c(...translatedMessage("close_6bf477426b"))}
          </Button>
          {!created && !receipt && (
            <Button
              variant="primary"
              disabled={
                busy ||
                !choices ||
                query.isError ||
                query.isFetching ||
                !initialRevision.current ||
                !!operation
              }
              onClick={() => void submit()}
            >
              {item
                ? c(...translatedMessage("save_6774e725d1"))
                : c(...translatedMessage("create_key_06433c30b0"))}
            </Button>
          )}
        </>
      }
    >
      {created ? (
        <div className="key-success">
          <div className="created-secret">
            <code dir="ltr">{created.key}</code>
            <Button
              onClick={() =>
                void copyText(
                  created.key,
                  c(...translatedMessage("copied_a2e05bbd2b")),
                  c(...translatedMessage("copy_failed_45c59affcc")),
                )
              }
            >
              {c(...translatedMessage("copy_4d8290cce9"))}
            </Button>
          </div>
          <p>
            {c(
              ...translatedMessage(
                "store_the_key_securely_you_can_view_it_again__3eb1a9b1f5",
              ),
            )}
          </p>
        </div>
      ) : (
        <div>
          {initialModel && (
            <p>
              {c(...translatedMessage("example_model_ec7557dd0f"))}
              <code>{initialModel}</code>
            </p>
          )}
          {reusable && (
            <div className="key-reuse">
              <p>{reusable.name}</p>
              <Button
                onClick={() => {
                  onClose();
                  navigate(
                    `/console/connect?key=${reusable.id}&model=${encodeURIComponent(initialModel!)}`,
                  );
                }}
              >
                {c(...translatedMessage("reuse_existing_key_c09b9dc867"))}
              </Button>
            </div>
          )}
          <form
            id={item ? "edit-key-form" : "new-key-form"}
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            <fieldset
              disabled={busy || !!operation || query.isFetching}
              style={{
                border: 0,
                padding: 0,
                minWidth: 0,
                display: "grid",
                gap: "1rem",
              }}
            >
              <label>
                {c(...translatedMessage("key_name_c60bdc543a"))}
                <Input
                  id={item ? "edit-key-name" : "new-key-name"}
                  value={name}
                  onChange={(e) =>
                    setName(Array.from(e.target.value).slice(0, 50).join(""))
                  }
                />
              </label>
              <label>
                {c(...translatedMessage("service_group_1c6f8adf41"))}
                <select
                  id="native-key-group"
                  className="select-trigger"
                  value={effectiveGroup}
                  onChange={(e) => {
                    setGroup(e.target.value);
                    setModelScope(
                      e.target.value === item?.tier ? "custom" : "snapshot",
                    );
                    setSnapshotModels(
                      choices?.choices
                        .filter((choice) => choice.group === e.target.value)
                        .map((choice) => choice.model) ?? [],
                    );
                    setPolicyDirty(e.target.value !== item?.tier);
                    setPolicyError(null);
                    setModels(
                      e.target.value === item?.tier
                        ? (item.allowedModels ?? null)
                        : null,
                    );
                  }}
                >
                  {!groups.includes(effectiveGroup) && (
                    <option value={effectiveGroup}>
                      {effectiveGroup || "—"}
                    </option>
                  )}
                  {groups.map((g) => (
                    <option key={g} value={g}>
                      {g}
                    </option>
                  ))}
                </select>
              </label>
              <NativeKeyModelPolicy
                choices={choices?.choices ?? []}
                group={effectiveGroup}
                available={available}
                selected={selected}
                scope={modelScope}
                locked={preservingAutoGroup}
                existing={!!item}
                legacyUnrestricted={!!item && item.modelAccess === "all"}
                onScopeChange={(scope) => {
                  setModelScope(scope);
                  if (scope === "snapshot") setSnapshotModels([...available]);
                  if (scope === "snapshot" && choices && !query.isFetching)
                    initialRevision.current = choices.revision;
                  setPolicyDirty(true);
                  setPolicyError(null);
                  setError("");
                }}
                onModelsChange={(next) => {
                  setModels(next);
                  setPolicyDirty(true);
                  if (candidateModels !== null)
                    setCandidateModels(
                      candidateModels.filter((model) => next.includes(model)),
                    );
                  setPolicyError(null);
                  setError("");
                }}
                smartEnabled={smartEnabled}
                onSmartChange={(enabled) => {
                  setSmartEnabled(enabled);
                  setSmartDirty(true);
                  setPolicyError(null);
                  setError("");
                }}
                smartAvailable={smartAvailable}
                smartSelected={smartSelected}
                onCandidatesChange={(next) => {
                  setCandidateModels(next);
                  setSmartDirty(true);
                  setPolicyError(null);
                  setError("");
                }}
                error={policyError}
                validationAttempt={validationAttempt}
              />
              <label>
                {c(
                  ...translatedMessage(
                    "spending_limit_usd_empty_for_unlimited_05b53bf257",
                  ),
                )}
                <Input
                  id="new-key-quota"
                  inputMode="decimal"
                  value={quota}
                  onChange={(e) => setQuota(e.target.value)}
                />
              </label>
              <label>
                {c(...translatedMessage("quota_period_23b646d127"))}
                <select
                  id="native-key-period"
                  className="select-trigger"
                  value={period}
                  onChange={(event) =>
                    setPeriod(event.target.value as typeof period)
                  }
                >
                  <option value="never">
                    {c(...translatedMessage("never_reset_6ac0732a7d"))}
                  </option>
                  <option value="daily">
                    {c(...translatedMessage("daily_890f87b664"))}
                  </option>
                  <option value="monthly">
                    {c(...translatedMessage("monthly_d84ce4da5f"))}
                  </option>
                </select>
                <small>
                  {c(
                    ...translatedMessage(
                      "resets_at_midnight_in_the_region_timezone_bc48ca5ea6",
                    ),
                  )}
                  {item?.quotaTimezone ??
                    choices?.quota_timezone ??
                    "Asia/Dubai"}
                </small>
              </label>
              <section className="native-key-access">
                <button
                  id="native-key-access-toggle"
                  type="button"
                  className="native-key-access-toggle"
                  aria-expanded={accessOpen}
                  aria-controls="native-key-access-fields"
                  onClick={() => setAccessOpen((open) => !open)}
                >
                  <span className="native-key-access-heading">
                    <strong>
                      {c(...translatedMessage("access_control_f7086ff9b1"))}
                    </strong>
                    <span>
                      {c(
                        ...translatedMessage(
                          "expiration_source_restrictions_optional_51f08e0895",
                        ),
                      )}
                    </span>
                  </span>
                  <ChevronDown size={15} aria-hidden="true" />
                </button>
                <div
                  id="native-key-access-fields"
                  className="native-key-access-body"
                  role="region"
                  aria-labelledby="native-key-access-toggle"
                  hidden={!accessOpen}
                >
                  <fieldset className="native-key-expiry">
                    <legend>
                      {c(...translatedMessage("expiration_cb80b02810"))}
                    </legend>
                    <div className="native-key-expiry-options">
                      {(["never", "date"] as const).map((mode) => (
                        <label key={mode} className="native-key-expiry-option">
                          <input
                            id={`native-key-expiry-${mode}`}
                            type="radio"
                            name="native-key-expiry-mode"
                            value={mode}
                            checked={expiryMode === mode}
                            onChange={() => {
                              setExpiryMode(mode);
                              setAccessError(null);
                            }}
                          />
                          <span>
                            {mode === "never"
                              ? c(...translatedMessage("no_expiry_89f56ad3fa"))
                              : c(
                                  ...translatedMessage(
                                    "set_expiration_8845c9b77a",
                                  ),
                                )}
                          </span>
                        </label>
                      ))}
                    </div>
                    {expiryMode === "date" && (
                      <label className="native-key-expiry-date">
                        <span>
                          {c(
                            ...translatedMessage(
                              "expiration_date_and_time_b7e2af4b90",
                            ),
                          )}
                        </span>
                        <Input
                          ref={expiryInput}
                          id="native-key-expiry"
                          type="datetime-local"
                          value={expiry}
                          aria-invalid={accessError === "expiry"}
                          aria-describedby={
                            accessError === "expiry"
                              ? "native-key-expiry-error"
                              : undefined
                          }
                          onChange={(event) => {
                            setExpiry(event.target.value);
                            setAccessError(null);
                          }}
                        />
                      </label>
                    )}
                    {accessError === "expiry" && (
                      <p
                        id="native-key-expiry-error"
                        className="native-key-access-error"
                        role="alert"
                      >
                        {c(
                          ...translatedMessage(
                            "choose_an_expiration_date_and_time_423aa894d1",
                          ),
                        )}
                      </p>
                    )}
                  </fieldset>
                  <div className="native-key-ip-field">
                    <div className="native-key-ip-heading">
                      <label htmlFor="native-key-ips">
                        {c(
                          ...translatedMessage(
                            "ip_source_address_allowlist_01c886470a",
                          ),
                        )}
                      </label>
                      <span
                        id="native-key-ip-count"
                        dir="ltr"
                        className={
                          ipEntries.length > 50 ? "is-over-limit" : undefined
                        }
                        aria-live="polite"
                        aria-atomic="true"
                      >
                        {ipEntries.length} / 50
                      </span>
                    </div>
                    <textarea
                      ref={ipInput}
                      id="native-key-ips"
                      dir="ltr"
                      className="input native-key-ip-input"
                      rows={4}
                      placeholder={
                        "203.0.113.10\n198.51.100.0/24\n2001:db8::/32"
                      }
                      value={ips}
                      aria-invalid={accessError === "ips"}
                      aria-describedby={`native-key-ip-hint native-key-ip-count${accessError === "ips" ? " native-key-ip-error" : ""}`}
                      onChange={(event) => {
                        setIps(event.target.value);
                        setAccessError(null);
                      }}
                    />
                    <p
                      id="native-key-ip-hint"
                      className="native-key-access-hint"
                    >
                      {c(
                        ...translatedMessage(
                          "one_entry_per_line_supports_ipv4_ipv6_and_cid_f46e3d939a",
                        ),
                      )}
                    </p>
                    {accessError === "ips" && (
                      <p
                        id="native-key-ip-error"
                        className="native-key-access-error"
                        role="alert"
                      >
                        {c(
                          ...translatedMessage(
                            "the_ip_allowlist_supports_up_to_50_entries_re_f372543be0",
                          ),
                        )}
                      </p>
                    )}
                  </div>
                </div>
              </section>
            </fieldset>
          </form>
          {!choices && query.isFetching && (
            <p role="status">
              {c(
                ...translatedMessage(
                  "loading_service_groups_and_models_3b9df8eead",
                ),
              )}
            </p>
          )}
          {choicesFailed && (
            <p role="alert">
              {c(
                ...translatedMessage(
                  "unable_to_load_service_groups_and_models_plea_81d71a1e15",
                ),
              )}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {operation && (
            <>
              <p>
                {c(
                  ...translatedMessage(
                    "the_result_is_unconfirmed_check_the_receipt_b_cd54bc96d7",
                  ),
                )}
              </p>
              <Button disabled={busy} onClick={() => void checkReceipt()}>
                {c(...translatedMessage("check_operation_receipt_7c28e8096d"))}
              </Button>
            </>
          )}
          {receipt && <p role="status">{receipt}</p>}
          {choicesFailed && (
            <Button
              id="native-key-choices-retry"
              disabled={busy || query.isFetching || !!operation}
              onClick={() => void query.refetch()}
            >
              {c(...translatedMessage("retry_c16455021c"))}
            </Button>
          )}
        </div>
      )}
    </Drawer>
  );
}
