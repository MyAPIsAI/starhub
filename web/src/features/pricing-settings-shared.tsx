import { message as translatedMessage } from "../i18n/messages";
import { useEffect, useRef, useState } from "react";
import { useSettingsText } from "./settings-text";
export { useSettingsText } from "./settings-text";
import { useModelPricing, validatePricing } from "../lib/model-pricing";
import { isAdmin } from "../lib/system-access";
import type { PricingCatalogConfig } from "../config/models";

/** Separate recoverable drafts for the two settings modules. Published revisions remain authoritative. */
export function usePricingDraft(
  module: "models" | "groups",
  role: number,
  scope?: string,
) {
  const config = useModelPricing((state) => state.config);
  const save = useModelPricing((state) => state.save);
  const storageKey = `veyra-settings-${module}-draft-v2${scope ? `-${scope}` : ""}`;
  const [initial] = useState(() => {
    try {
      const raw = sessionStorage.getItem(storageKey);
      if (raw) {
        const stored = JSON.parse(raw) as {
          base: PricingCatalogConfig;
          draft: PricingCatalogConfig;
        };
        if (
          validatePricing(stored.base) &&
          stored.draft?.models &&
          stored.draft?.multipliers &&
          stored.draft?.groups &&
          (!scope || (stored.base.models[scope] && stored.draft.models[scope]))
        )
          return stored;
      }
    } catch {
      /* Browser may deny draft storage. */
    }
    return { base: config, draft: structuredClone(config) };
  });
  const [base, setBase] = useState(initial.base);
  const [draft, setDraft] = useState(initial.draft);
  const [notice, setNotice] = useState("");
  const [reviewOpen, setReviewOpen] = useState(false);
  const dirty = JSON.stringify(base) !== JSON.stringify(draft);
  const conflict = base.revision !== config.revision;
  const previous = useRef(config);
  const { text } = useSettingsText();
  useEffect(() => {
    const reset = previous.current !== config && config.updatedAt === null;
    previous.current = config;
    if (!dirty || reset) {
      setBase(config);
      setDraft(structuredClone(config));
      if (reset) {
        setNotice("");
        setReviewOpen(false);
      }
    }
  }, [config, dirty]);
  useEffect(() => {
    try {
      if (dirty)
        sessionStorage.setItem(storageKey, JSON.stringify({ base, draft }));
      else sessionStorage.removeItem(storageKey);
    } catch {
      /* Editing still works; no claim that browser persisted the draft. */
    }
  }, [base, draft, dirty, storageKey]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  const discard = () => {
    setBase(config);
    setDraft(structuredClone(config));
    setNotice("");
    setReviewOpen(false);
  };
  const publish = () => {
    const result = save(draft, role, base.revision);
    if (!result.ok) {
      setNotice(
        result.reason === "conflict"
          ? text(
              ...translatedMessage(
                "a_newer_version_was_published_discard_this_dr_8c7296a96f",
              ),
            )
          : text(
              ...translatedMessage(
                "not_published_check_values_administrator_acce_804d2c5aa6",
              ),
            ),
      );
      return false;
    }
    setBase(result.config);
    setDraft(structuredClone(result.config));
    setReviewOpen(false);
    setNotice(
      text(
        ...translatedMessage(
          "saved_to_this_browser_s_demo_pricing_future_s_0ba87d14bb",
        ),
      ),
    );
    return true;
  };
  return {
    config,
    base,
    draft,
    setDraft,
    notice,
    setNotice,
    reviewOpen,
    setReviewOpen,
    dirty,
    conflict,
    discard,
    publish,
    valid: validatePricing(draft),
    canEdit: isAdmin(role),
    text,
  };
}
