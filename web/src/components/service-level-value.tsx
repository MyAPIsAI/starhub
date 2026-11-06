import { message as translatedMessage } from "../i18n/messages";
import { readServiceLevel, successPercent } from "../lib/service-level";
import "./service-level-value.css";

export function ServiceLevelValue({
  value,
  text,
}: {
  value: unknown;
  text: (zh: string, en: string, ar: string) => string;
}) {
  const sla = readServiceLevel(value);
  if (value === null || sla?.targets === null)
    return (
      <span>{text(...translatedMessage("not_published_cc5c3ba13c"))}</span>
    );
  if (!sla?.targets)
    return (
      <span>
        {text(...translatedMessage("targets_unavailable_a62eb2f6f2"))}
      </span>
    );
  const target = sla.targets;
  return (
    <div className="service-level-value">
      <span>
        {text(...translatedMessage("success_target_0d6cdcf9cf"))}:{" "}
        <bdi>
          {target.success_rate_ppm === null
            ? "—"
            : `${successPercent(target.success_rate_ppm)}%`}
        </bdi>
      </span>
      <span>
        {text(...translatedMessage("p95_target_a1283236a8"))}:{" "}
        <bdi>
          {target.p95_latency_ms === null ? "—" : `${target.p95_latency_ms} ms`}
        </bdi>
      </span>
    </div>
  );
}
