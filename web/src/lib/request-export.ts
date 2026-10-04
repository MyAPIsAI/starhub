import type { PreviewKey, PreviewRequest } from "./product-preview";
import type { MeteredUsage } from "./pricing-rules";

export function filterExportRequests(rows: PreviewRequest[], range: {startDate: string; endDate: string; createdAt: string}) {
  const from = Date.parse(`${range.startDate.slice(0, 10)}T00:00:00Z`);
  const to = Math.min(Date.parse(`${range.endDate.slice(0, 10)}T00:00:00Z`) + 86400000 - 1, Date.parse(range.createdAt));
  return rows.filter(row => Date.parse(row.time) >= from && Date.parse(row.time) <= to);
}
const dollars = (value: string) => { const nano = BigInt(value); return `${nano / 1000000000n}.${(nano % 1000000000n).toString().padStart(9, "0")}`; };
const csvCell = (value: unknown) => {
  const raw = String(value ?? "");
  const safe = (/^[\s\u0000-\u001f\u007f-\u009f]*[=+\-@]/u.test(raw) || /^[\t\r\n]/.test(raw)) ? `'${raw}` : raw;
  return `"${safe.replaceAll('"', '""')}"`;
};
const csv = (rows: unknown[][]) => "\uFEFF" + rows.map(row => row.map(csvCell).join(",")).join("\r\n");
const billedUsage = (row: PreviewRequest): MeteredUsage => row.status === "success" ? row.metered_usage ?? row.pricing_snapshot?.usage ?? { input_tokens: row.input_tokens, output_tokens: row.output_tokens, requests: 1 } : {};

export function aggregateRequestUsage(rows: PreviewRequest[]) {
  const groups = new Map<string, {date: string; model: string; tier: string; mode: string; requests: number; failed: number; amountNano: string; usage: MeteredUsage}>();
  for (const row of rows) {
    const date = new Date(row.time).toISOString().slice(0, 10), model = row.actual_model ?? row.requested_model ?? row.model;
    const rule = row.pricing_snapshot?.rule;
    const mode = rule?.mode === "unit" ? rule.unit : rule?.mode ?? (row.metered_demo ? "unsettled" : "tokens");
    const id = JSON.stringify([date, model, row.tier, mode]);
    const group = groups.get(id) ?? { date, model, tier: row.tier, mode, requests: 0, failed: 0, amountNano: "0", usage: {} };
    group.requests++; group.failed += row.status === "failed" ? 1 : 0;
    group.amountNano = (BigInt(group.amountNano) + BigInt(row.cost_nano)).toString();
    for (const [field, value] of Object.entries(billedUsage(row))) {
      const key = field as keyof MeteredUsage;
      group.usage[key] = Number(((group.usage[key] ?? 0) + (value ?? 0)).toFixed(12));
    }
    groups.set(id, group);
  }
  return [...groups.values()];
}

export function serializeRequestExport(rows: PreviewRequest[], keys: PreviewKey[]) {
  return csv([
    ["Time UTC（协调世界时）", "Trace ID（业务记录）", "Key ID（密钥标识）", "Key name（密钥名称）", "Requested model（请求模型）", "Actual model（实际模型）", "Status（状态）", "Billing status（计费状态）", "Error code（错误代码）", "Pricing rule（计价规则）", "Submitted usage（提交用量）", "Cost USD（美元费用）", "Cost nano-USD（纳美元费用）", "Pricing revision（价格版本）", "Pricing snapshot（计价快照）"],
    ...rows.map(row => [row.time, row.trace_id, row.key_id, keys.find(key => key.id === row.key_id)?.name ?? "", row.requested_model ?? row.model, row.actual_model ?? (row.status === "success" ? row.model : ""), row.status, row.billing_status ?? (row.status === "success" ? "charged" : "not_charged"), row.error_code ?? "", row.pricing_snapshot ? JSON.stringify(row.pricing_snapshot.rule) : "", JSON.stringify(row.metered_usage ?? row.pricing_snapshot?.usage ?? billedUsage(row)), dollars(row.cost_nano), row.cost_nano, row.pricing_revision, row.pricing_snapshot ? JSON.stringify(row.pricing_snapshot) : ""]),
  ]);
}
export function serializeUsageExport(rows: PreviewRequest[]) {
  return csv([
    ["Date UTC（协调世界时日期）", "Model（模型）", "Group（服务分组）", "Billing mode（计费方式）", "Requests（调用数）", "Failures（失败数）", "Settled usage（已结算用量）", "Cost USD（美元费用）", "Cost nano-USD（纳美元费用）"],
    ...aggregateRequestUsage(rows).map(row => [row.date, row.model, row.tier, row.mode, row.requests, row.failed, JSON.stringify(row.usage), dollars(row.amountNano), row.amountNano]),
  ]);
}
