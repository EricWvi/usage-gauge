import type { Sample, Tier } from "./types.ts";

export const HOUR = 3_600_000;
export const colors = [
  "var(--chart-1)",
  "var(--chart-2)",
  "var(--chart-3)",
  "var(--chart-4)",
  "var(--chart-5)",
];
export function percent(tier: Tier) {
  const n =
    tier.utilization ||
    (tier.limit ? ((tier.used ?? 0) / tier.limit) * 100 : 0);
  return Math.max(0, Math.min(100, n));
}
export function tierLabel(tier: Tier) {
  return (tier.label || tier.name)
    .replace(/five_hour/g, "5-hour window")
    .replace(/weekly/g, "Weekly window")
    .replace(/(\d+)_minutes/g, "$1-minute window")
    .replace(/_/g, " ");
}
export function seriesFor(history: Sample[], latest: Sample | null) {
  const tiers = new Map<string, Tier>();
  for (const sample of history)
    for (const tier of sample.tiers) tiers.set(tier.name, tier);
  for (const tier of latest?.tiers ?? []) tiers.set(tier.name, tier);
  return [...tiers.values()].map((tier, i) => ({
    tier,
    key: `s${i}`,
    color: colors[i % colors.length],
  }));
}
export type Series = ReturnType<typeof seriesFor>;
export type Point = { time: number; [key: string]: number | null };

// Missing samples and quota resets are discontinuities, never interpolated trends.
export function chartPoints(
  history: Sample[],
  series: Series,
  interval: number,
): Point[] {
  const points: Point[] = [];
  let previous: Sample | undefined;
  for (const sample of history) {
    const point: Point = { time: sample.updatedAt };
    const gap: Point = {
      time: previous ? previous.updatedAt + 1 : sample.updatedAt,
    };
    let discontinuity = false;
    for (const item of series) {
      const current = sample.tiers.find((t) => t.name === item.tier.name);
      const prev = previous?.tiers.find((t) => t.name === item.tier.name);
      const value = sample.status === "ok" && current ? percent(current) : null;
      point[item.key] = value;
      const reset = current && prev && percent(current) < percent(prev);
      const missing =
        previous && sample.updatedAt - previous.updatedAt > interval * 1.5;
      const breakLine = reset || missing;
      gap[item.key] = breakLine
        ? null
        : previous?.status === "ok" && prev
          ? percent(prev)
          : null;
      discontinuity ||= !!breakLine;
    }
    if (discontinuity) points.push(gap);
    points.push(point);
    previous = sample;
  }
  return points;
}

export function visibleRange(now: number, hours: number): [number, number] {
  return [now - hours * HOUR, now];
}
