import type { StatusDay } from "./api/public-pages";

export function statusPeriod(days: StatusDay[]) {
  const samples = days.reduce((sum, day) => sum + day.samples, 0);
  const complete = days.every((day) => Number.isFinite(day.successful));
  const successful = days.reduce((sum, day) => sum + (day.successful ?? 0), 0);
  return {
    samples,
    successRate: complete && samples > 0 ? successful / samples : null,
  };
}
