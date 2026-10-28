export interface ServiceLevelTargets {
  success_rate_ppm: number | null;
  p95_latency_ms: number | null;
}

export interface ServiceLevel {
  revision: number;
  targets: ServiceLevelTargets | null;
  updated_at: number;
}

const integer = (value: unknown, min: number, max: number): value is number =>
  typeof value === "number" &&
  Number.isSafeInteger(value) &&
  value >= min &&
  value <= max;

// Missing or malformed metadata is unknown, never an editable initial revision.
export function readServiceLevel(value: unknown): ServiceLevel | undefined {
  if (!value || typeof value !== "object") return undefined;
  const sla = value as Record<string, unknown>;
  if (
    !integer(sla.revision, 0, Number.MAX_SAFE_INTEGER) ||
    !integer(sla.updated_at, 0, Number.MAX_SAFE_INTEGER)
  )
    return undefined;
  if (sla.targets === null)
    return {
      revision: sla.revision,
      updated_at: sla.updated_at,
      targets: null,
    };
  if (
    !sla.targets ||
    typeof sla.targets !== "object" ||
    sla.revision < 1 ||
    sla.updated_at < 1
  )
    return undefined;
  const target = sla.targets as Record<string, unknown>;
  const success = target.success_rate_ppm;
  const latency = target.p95_latency_ms;
  if (
    (success !== null && !integer(success, 0, 1_000_000)) ||
    (latency !== null && !integer(latency, 1, 86_400_000)) ||
    (success === null && latency === null)
  )
    return undefined;
  return {
    revision: sla.revision,
    updated_at: sla.updated_at,
    targets: { success_rate_ppm: success, p95_latency_ms: latency },
  };
}

export function successPercent(ppm: number): string {
  const fraction = String(ppm % 10_000)
    .padStart(4, "0")
    .replace(/0+$/, "");
  return `${Math.floor(ppm / 10_000)}${fraction ? `.${fraction}` : ""}`;
}

// Parse fixed-point percentages without rounding a published target.
export function parseServiceLevelTargets(
  percent: string,
  milliseconds: string,
): ServiceLevelTargets | undefined {
  const success = percent.trim();
  const latency = milliseconds.trim();
  if (
    (!success && !latency) ||
    (success && !/^\d{1,3}(\.\d{1,4})?$/.test(success)) ||
    (latency && !/^\d{1,8}$/.test(latency))
  )
    return undefined;
  const [whole, fraction = ""] = success.split(".");
  const ppm = success
    ? Number(whole) * 10_000 + Number(fraction.padEnd(4, "0"))
    : null;
  const ms = latency ? Number(latency) : null;
  if (
    (ppm !== null && !integer(ppm, 0, 1_000_000)) ||
    (ms !== null && !integer(ms, 1, 86_400_000))
  )
    return undefined;
  return { success_rate_ppm: ppm, p95_latency_ms: ms };
}
