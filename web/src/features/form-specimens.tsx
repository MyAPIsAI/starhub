import { useState } from "react";
import * as Slider from "@radix-ui/react-slider";
import { Check, ArrowRight, Bell, Trash2, CalendarDays } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Button } from "../components/ui/button";
import {
  Field,
  Input,
  Select,
  Switch,
  Badge,
  Progress,
  Tooltip,
} from "../components/ui/primitives";
import { placeholders } from "../config/placeholders";
import { usePreferences } from "../lib/preferences";

export function FormSpecimens({ onConfirm }: { onConfirm: () => void }) {
  const { t } = useTranslation();
  const language = usePreferences((s) => s.language);
  const [name, setName] = useState("");
  const [error, setError] = useState(false);
  const [tier, setTier] = useState("pro");
  const [stream, setStream] = useState(true);
  const [temperature, setTemperature] = useState([0.7]);
  const [dateFrom, setDateFrom] = useState("2026-09-12");
  const [dateTo, setDateTo] = useState("2026-09-18");
  const [dateError, setDateError] = useState("");
  return (
    <div className="specimen-columns">
      <div className="specimen-block">
        <h3>{t("buttonsSection")}</h3>
        <div className="button-specimens">
          <Button
            variant="primary"
            onClick={() =>
              toast.success(t("toastSuccess"), {
                description: t("toastDescription"),
              })
            }
          >
            {t("primary")}
            <ArrowRight className="directional" />
          </Button>
          <Button onClick={() => toast(t("toastDescription"))}>
            {t("secondary")}
          </Button>
          <Button variant="ghost" onClick={() => toast(t("toastDescription"))}>
            {t("quiet")}
          </Button>
          <Button variant="danger" onClick={onConfirm}>
            <Trash2 />
            {t("danger")}
          </Button>
          <Tooltip text={t("componentDisabled")}>
            <span tabIndex={0} className="disabled-wrapper">
              <Button disabled>{t("disabled")}</Button>
            </span>
          </Tooltip>
          <Button loading>{t("busy")}</Button>
        </div>
        <div className="status-specimens">
          <Badge tone="success" dot>
            {t("healthy")}
          </Badge>
          <Badge tone="warning" dot>
            {t("warning")}
          </Badge>
          <Badge tone="danger" dot>
            {t("failed")}
          </Badge>
          <Badge>{t("notEnough")}</Badge>
        </div>
        <div className="quota-example">
          <div>
            <span>{t("quota")}</span>
            <b className="numeric ltr">92%</b>
          </div>
          <Progress value={92} label={t("quota")} />
          <p>{t("quotaUsed", { used: "$92.00", limit: "$100.00" })}</p>
          <small>{t("quotaHint")}</small>
        </div>
        <div className="date-example">
          <h3>
            <CalendarDays size={16} />
            {t("dateRange")}
          </h3>
          <div className="date-inputs">
            <Field id="from" label={t("fromDate")}>
              <Input
                type="date"
                id="from"
                value={dateFrom}
                onChange={(e) => {
                  setDateFrom(e.target.value);
                  setDateError("");
                }}
                aria-describedby={dateError ? "date-error" : undefined}
              />
            </Field>
            <Field id="to" label={t("toDate")}>
              <Input
                type="date"
                id="to"
                value={dateTo}
                onChange={(e) => {
                  setDateTo(e.target.value);
                  setDateError("");
                }}
                aria-describedby={dateError ? "date-error" : undefined}
              />
            </Field>
          </div>
          {dateError && (
            <p className="field-error" role="alert" id="date-error">
              {t(dateError)}
            </p>
          )}
          <Button
            onClick={() => {
              const days =
                (Date.parse(dateTo) - Date.parse(dateFrom)) / 86400000 + 1;
              const message =
                !dateFrom || !dateTo
                  ? "invalidRange"
                  : days < 1
                    ? "rangeInvalid"
                    : days > 31
                      ? "exportLimit"
                      : "";
              setDateError(message);
              if (!message) toast.success(t("rangeValid"));
            }}
          >
            {t("applyRange")}
          </Button>
        </div>
      </div>
      <form
        className="specimen-block form-specimen"
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim()) {
            setError(true);
            document.getElementById("configuration-name")?.focus();
          } else {
            setError(false);
            toast.success(t("validated"), { description: t("formSaved") });
          }
        }}
        noValidate
      >
        <h3>{t("fieldGroup")}</h3>
        <Field
          id="configuration-name"
          label={t("fieldName")}
          hint={t("fieldHelp")}
          error={error ? t("required") : undefined}
        >
          <Input
            id="configuration-name"
            placeholder={t("fieldPlaceholder")}
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setError(false);
            }}
            maxLength={50}
            aria-invalid={error}
            aria-describedby="configuration-name-help"
          />
        </Field>
        <Field id="tier-select" label={t("tier")}>
          <Select
            id="tier-select"
            value={tier}
            onChange={setTier}
            label={t("selectTier")}
            options={placeholders.tiers.map((item) => ({
              value: item.id,
              label: item.name[language],
            }))}
          />
        </Field>
        <div className="switch-row">
          <div>
            <label htmlFor="stream-output">{t("streaming")}</label>
            <p>{t("streamingHelp")}</p>
          </div>
          <Switch
            checked={stream}
            onChange={setStream}
            id="stream-output"
            label={t("streaming")}
          />
        </div>
        <div className="slider-field">
          <div>
            <label id="temperature-label">{t("temperature")}</label>
            <output className="numeric ltr">{temperature[0].toFixed(1)}</output>
          </div>
          <Slider.Root
            className="slider"
            value={temperature}
            onValueChange={setTemperature}
            min={0}
            max={2}
            step={0.1}
            dir={language === "ar" ? "rtl" : "ltr"}
            aria-labelledby="temperature-label"
          >
            <Slider.Track className="slider-track">
              <Slider.Range className="slider-range" />
            </Slider.Track>
            <Slider.Thumb
              className="slider-thumb"
              aria-label={t("temperature")}
            />
          </Slider.Root>
        </div>
        <Field id="readonly" label={t("nameDisabled")}>
          <Input id="readonly" value={t("readonlyValue")} disabled />
        </Field>
        <div className="form-footer">
          <Button type="submit" variant="primary">
            <Check />
            {t("validate")}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("notify")}
            onClick={() => toast.success(t("toastSuccess"))}
          >
            <Bell />
          </Button>
        </div>
      </form>
    </div>
  );
}
