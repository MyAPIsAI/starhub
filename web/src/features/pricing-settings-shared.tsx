import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useModelPricing, validatePricing } from "../lib/model-pricing";
import type { PricingCatalogConfig } from "../config/models";

export function useSettingsText() {
  const { i18n } = useTranslation();
  const lang: "zh" | "ar" | "en" = i18n.language.startsWith("zh")
    ? "zh"
    : i18n.language.startsWith("ar")
      ? "ar"
      : "en";
  return {
    lang,
    text: (zh: string, en: string, ar: string) =>
      lang === "zh" ? zh : lang === "ar" ? ar : en,
  };
}

/** Separate recoverable drafts for the two settings modules. Published revisions remain authoritative. */
export function usePricingDraft(module: "models" | "groups", role: string, scope?: string) {
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
              "另一页面已发布新版本。请放弃草稿，加载最新配置后重新修改。",
              "A newer version was published. Discard this draft and reload before editing.",
              "نُشر إصدار أحدث. تجاهل المسودة وأعد التحميل قبل التعديل.",
            )
          : text(
              "未发布。请检查输入、管理员身份和浏览器存储权限，草稿已保留。",
              "Not published. Check values, administrator access and browser storage. Your draft is retained.",
              "لم يتم النشر. تحقق من القيم والصلاحية والتخزين. احتُفظ بالمسودة.",
            ),
      );
      return false;
    }
    setBase(result.config);
    setDraft(structuredClone(result.config));
    setReviewOpen(false);
    setNotice(
      text(
        "已发布并生效。后续演示调用使用新配置，历史调用与账单保留原费率。",
        "Published. Future demo calls use the new configuration; historical requests and bills retain their rates.",
        "تم النشر. تستخدم الطلبات القادمة الإعداد الجديد وتحتفظ الفواتير السابقة بأسعارها.",
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
    canEdit: role === "superAdmin",
    text,
  };
}
