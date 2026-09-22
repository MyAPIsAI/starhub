export interface User {
  id: number;
  username: string;
  email: string;
  role: number;
  status: number;
  quota: number;
  used_quota: number;
  group: string;
  aff_code: string;
  inviter_id: number;
  created_at: number;
  request_count?: number;
}
export type Row = Record<
  string,
  string | number | boolean | null | undefined | Record<string, unknown>
> & { id: number };
export interface PageResult {
  items: Row[] | null;
  total: number;
}
export interface DailyUsage {
  date: string;
  requests: number;
  quota: number;
  unfunded_quota: number;
  prompt_tokens: number;
  completion_tokens: number;
}
export interface Usage {
  days: number;
  items: DailyUsage[] | null;
}
export interface ChannelStat {
  channel_id: number;
  samples: number;
  success_rate: number;
  avg_ttft_ms: number;
  p95_ttft_ms: number;
  avg_latency_ms: number;
}
export interface Metrics {
  requests: { class: string; count: number }[];
  channel_stats: ChannelStat[] | null;
  window: number;
  generated_at: number;
}
export interface Option {
  key: string;
  value: string;
}
