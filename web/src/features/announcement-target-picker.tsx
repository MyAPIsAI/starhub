import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/primitives";
import { apiGet } from "../lib/api/http";
import { useStaffText } from "../lib/api/staff";
import { useSessionQueryIdentity } from "../lib/system-access";

const targetLimit = 40;
interface CatalogModel {
  model: string;
  name: string;
  vendor: string;
  listed: boolean;
}
interface TargetOption {
  value: string;
  detail?: string;
  unavailable?: boolean;
}

function TargetChecklist({
  kind,
  title,
  placeholder,
  options,
  selected,
  remaining,
  disabled,
  onChange,
}: {
  kind: "vendors" | "models";
  title: string;
  placeholder: string;
  options: TargetOption[];
  selected: string[];
  remaining: number;
  disabled: boolean;
  onChange: (values: string[]) => void;
}) {
  const t = useStaffText();
  const [search, setSearch] = useState("");
  const needle = search.trim().toLowerCase();
  const visible = options.filter((option) =>
    `${option.value} ${option.detail ?? ""}`.toLowerCase().includes(needle),
  );
  const additions = visible.filter(
    (option) => !selected.includes(option.value),
  );
  return (
    <fieldset className="announcement-target-picker" data-target-picker={kind}>
      <legend>{title}</legend>
      <Input
        id={`notice-${kind}-search`}
        aria-label={placeholder}
        placeholder={placeholder}
        type="search"
        value={search}
        onChange={(event) => setSearch(event.target.value)}
      />
      <div className="announcement-target-toolbar">
        <span aria-live="polite">
          {t(...translatedMessage("selected_a99ed4262a"))} {selected.length}
        </span>
        <Button
          size="small"
          variant="ghost"
          disabled={
            disabled || !additions.length || additions.length > remaining
          }
          onClick={() =>
            onChange([...selected, ...additions.map((v) => v.value)])
          }
        >
          {t(...translatedMessage("select_results_fac162c83b"))}
        </Button>
        <Button
          size="small"
          variant="ghost"
          disabled={disabled || !selected.length}
          onClick={() => onChange([])}
        >
          {t(...translatedMessage("clear_selection_2f09980638"))}
        </Button>
      </div>
      <div className="announcement-target-options">
        {visible.map((option) => {
          const checked = selected.includes(option.value);
          return (
            <label className="announcement-target-option" key={option.value}>
              <input
                type="checkbox"
                value={option.value}
                checked={checked}
                disabled={disabled || (!checked && remaining <= 0)}
                onChange={(event) =>
                  onChange(
                    event.target.checked
                      ? [...selected, option.value]
                      : selected.filter((value) => value !== option.value),
                  )
                }
              />
              <span>
                <bdi>{option.value}</bdi>
                {option.detail && <small>{option.detail}</small>}
                {option.unavailable && (
                  <small>
                    {t(
                      ...translatedMessage(
                        "no_longer_in_the_catalog_existing_selection_r_9b1ac8649c",
                      ),
                    )}
                  </small>
                )}
              </span>
            </label>
          );
        })}
        {!visible.length && !disabled && (
          <p>
            {needle
              ? t(...translatedMessage("no_matches_a9ca92c804"))
              : t(...translatedMessage("no_catalog_entries_c0459a0250"))}
          </p>
        )}
      </div>
    </fieldset>
  );
}

export function AnnouncementTargetPicker({
  vendors,
  models,
  disabled,
  onChange,
}: {
  vendors: string[];
  models: string[];
  disabled: boolean;
  onChange: (selection: { vendors: string[]; models: string[] }) => void;
}) {
  const t = useStaffText();
  const identity = useSessionQueryIdentity();
  const catalog = useQuery({
    queryKey: ["announcement-target-catalog", ...identity],
    queryFn: ({ signal }) =>
      apiGet<{ models: CatalogModel[] }>("/api/admin/models", signal),
    staleTime: 0,
    retry: false,
  });
  const entries = catalog.isError ? [] : (catalog.data?.models ?? []);
  const modelOptions: TargetOption[] = entries.map((entry) => ({
    value: entry.model,
    detail: [
      entry.name !== entry.model ? entry.name : "",
      entry.vendor,
      !entry.listed ? t(...translatedMessage("unlisted_b2b4094301")) : "",
    ]
      .filter(Boolean)
      .join(" · "),
  }));
  const vendorOptions: TargetOption[] = [
    ...new Set(entries.map((entry) => entry.vendor).filter(Boolean)),
  ].map((value) => ({ value }));
  const retainSelection = (options: TargetOption[], selected: string[]) => {
    const unique = new Map(options.map((option) => [option.value, option]));
    for (const value of selected) {
      if (!unique.has(value))
        unique.set(value, { value, unavailable: catalog.isSuccess });
    }
    return [...unique.values()].sort((a, b) => a.value.localeCompare(b.value));
  };
  const remaining = targetLimit - vendors.length - models.length;
  const locked = disabled || catalog.isPending || catalog.isError;
  return (
    <section className="announcement-target-section">
      <div className="announcement-target-toolbar">
        <p>
          {t(
            ...translatedMessage(
              "choose_vendors_or_models_from_the_platform_ca_72bcb0dc2f",
              { p0: targetLimit },
            ),
          )}
        </p>
        <Button
          size="small"
          disabled={disabled || catalog.isFetching}
          onClick={() => void catalog.refetch()}
        >
          {t(...translatedMessage("refresh_catalog_7ee4cb22ea"))}
        </Button>
      </div>
      {catalog.isPending && (
        <p role="status">
          {t(...translatedMessage("loading_model_catalog_a6f85e62c3"))}
        </p>
      )}
      {catalog.isError && (
        <p role="alert">
          {t(
            ...translatedMessage(
              "unable_to_load_the_model_catalog_refresh_to_r_be859bf352",
            ),
          )}
        </p>
      )}
      <div className="announcement-targets">
        <TargetChecklist
          kind="vendors"
          title={t(...translatedMessage("affected_vendors_243bf5e809"))}
          placeholder={t(...translatedMessage("search_vendors_dc7c88dd74"))}
          options={retainSelection(vendorOptions, vendors)}
          selected={vendors}
          remaining={remaining}
          disabled={locked}
          onChange={(values) => onChange({ vendors: values, models })}
        />
        <TargetChecklist
          kind="models"
          title={t(...translatedMessage("affected_models_25e494c190"))}
          placeholder={t(
            ...translatedMessage(
              "search_model_names_ids_or_vendors_a1b22d6b15",
            ),
          )}
          options={retainSelection(modelOptions, models)}
          selected={models}
          remaining={remaining}
          disabled={locked}
          onChange={(values) => onChange({ vendors, models: values })}
        />
      </div>
    </section>
  );
}
