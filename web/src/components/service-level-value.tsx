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
      <span>{text("目标未发布", "Not published", "لم تُنشر الأهداف")}</span>
    );
  if (!sla?.targets)
    return (
      <span>
        {text("目标未知", "Targets unavailable", "الأهداف غير متاحة")}
      </span>
    );
  const target = sla.targets;
  return (
    <div className="service-level-value">
      <span>
        {text("成功率目标", "Success target", "نسبة النجاح المستهدفة")}:{" "}
        <bdi>
          {target.success_rate_ppm === null
            ? "—"
            : `${successPercent(target.success_rate_ppm)}%`}
        </bdi>
      </span>
      <span>
        {text("P95 目标", "P95 target", "هدف P95")}:{" "}
        <bdi>
          {target.p95_latency_ms === null ? "—" : `${target.p95_latency_ms} ms`}
        </bdi>
      </span>
    </div>
  );
}
