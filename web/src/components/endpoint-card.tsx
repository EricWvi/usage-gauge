import { useId, useMemo, useState } from "react";
import { Activity, Clock3, Gauge, TriangleAlert } from "lucide-react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import type { Endpoint, Tier } from "@/lib/types";
import {
  chartPoints,
  HOUR,
  percent,
  seriesFor,
  tierLabel,
  visibleRange,
} from "@/lib/history";
import { clock, dateTime, pct } from "@/lib/format";

function resetText(tier: Tier, now: number) {
  const reset = Date.parse(tier.resetsAt ?? "");
  if (!Number.isFinite(reset)) return "Reset time unavailable";
  const minutes = Math.ceil((reset - now) / 60_000);
  if (minutes <= 0) return "Reset due · awaiting sample";
  if (minutes < 60) return `Resets in ${minutes}m`;
  if (minutes < 1440)
    return `Resets in ${Math.floor(minutes / 60)}h ${minutes % 60}m`;
  return `Resets in ${Math.floor(minutes / 1440)}d ${Math.floor((minutes % 1440) / 60)}h`;
}

export function EndpointCard({
  endpoint,
  now,
  interval,
}: {
  endpoint: Endpoint;
  now: number;
  interval: number;
}) {
  const [hours, setHours] = useState(24);
  const id = useId().replace(/[^a-zA-Z0-9]/g, "");
  const series = useMemo(
    () => seriesFor(endpoint.history, endpoint.latest),
    [endpoint.history, endpoint.latest],
  );
  const points = useMemo(
    () => chartPoints(endpoint.history, series, interval),
    [endpoint.history, series, interval],
  );
  const [start, end] = visibleRange(now, hours);
  const config = Object.fromEntries(
    series.map((s) => [s.key, { label: tierLabel(s.tier), color: s.color }]),
  ) satisfies ChartConfig;
  const latest = endpoint.latest;
  const stale = latest && now - latest.updatedAt > interval * 2;
  const healthy = latest?.status === "ok" && !stale;
  const status = !latest
    ? "Waiting"
    : stale
      ? "Delayed"
      : latest.status === "ok"
        ? "Connected"
        : latest.status === "expired"
          ? "Auth expired"
          : "Query failed";
  const goodPoints = endpoint.history.filter(
    (s) =>
      s.status === "ok" &&
      s.tiers.length &&
      s.updatedAt >= start &&
      s.updatedAt <= end,
  );
  const visiblePoints = points.filter((p) => p.time >= start && p.time <= end);
  const provider =
    endpoint.provider === "codex"
      ? "OpenAI"
      : endpoint.provider === "zai"
        ? "Z.ai"
        : endpoint.provider === "claude"
          ? "Anthropic"
          : endpoint.provider;

  return (
    <Card
      className="endpoint-card overflow-hidden rounded-2xl border-border/80 py-0 shadow-sm"
      data-endpoint={endpoint.name}
    >
      <CardContent className="p-0">
        <div className="flex items-start justify-between gap-3 px-5 pt-6 sm:px-7">
          <div className="flex min-w-0 items-center gap-3.5">
            <div
              className={`provider-icon ${endpoint.provider === "zai" ? "provider-zai" : ""}`}
              aria-hidden="true"
            >
              {endpoint.provider === "zai" ? (
                <span>
                  Z<span className="text-primary">.</span>
                </span>
              ) : (
                <Gauge size={23} strokeWidth={1.7} />
              )}
            </div>
            <div className="min-w-0">
              <h2 className="truncate text-lg font-semibold tracking-tight">
                {endpoint.name}
              </h2>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {provider}
                {latest?.message ? ` / ${latest.message}` : " / Usage overview"}
              </p>
            </div>
          </div>
          <Badge
            variant="outline"
            className={`mt-1 gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-medium ${healthy ? "status-healthy" : "text-muted-foreground"}`}
          >
            <span
              className={`size-1.5 rounded-full ${healthy ? "bg-primary" : latest ? "bg-amber-500" : "bg-muted-foreground"}`}
            />
            {status}
          </Badge>
        </div>

        {latest?.error && (
          <div
            role="alert"
            className="mx-5 mt-5 flex items-start gap-2 rounded-lg border border-destructive/20 bg-destructive/5 p-3 text-xs text-destructive sm:mx-7"
          >
            <TriangleAlert className="size-4 shrink-0" />
            <span className="break-all">{latest.error}</span>
          </div>
        )}
        <div className="quota-grid mx-5 my-6 sm:mx-7">
          {latest?.status === "ok" && latest.tiers.length > 0 ? (
            latest.tiers.map((tier) => {
              const item = series.find((s) => s.tier.name === tier.name);
              const used = percent(tier);
              const weekly = /\bweekly\b/i.test(tier.label || tier.name);
              const value = weekly ? 100 - used : used;
              const color = weekly
                ? `hsl(${value * 1.2} 65% 45%)`
                : used >= 90
                  ? "var(--destructive)"
                  : item?.color;
              return (
                <div
                  className="quota-metric"
                  key={tier.name}
                  data-tier={tier.name}
                >
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span
                      className="size-2 rounded-full"
                      style={{ background: item?.color }}
                    />
                    {tierLabel(tier)}
                  </div>
                  <div className="mt-2 flex items-baseline gap-2">
                    <span
                      className="text-[32px] font-semibold leading-tight tracking-[-1.5px] tabular-nums"
                      style={weekly ? { color } : undefined}
                      data-testid="quota-value"
                    >
                      {pct(value)}
                      <span className="ml-0.5 text-xl font-normal text-muted-foreground">
                        %
                      </span>
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {weekly ? "left" : "used"}
                    </span>
                    {weekly ? (
                      <span
                        className="ml-auto flex items-center gap-1.5 text-xs tabular-nums text-muted-foreground"
                        title={
                          tier.resetsAt
                            ? dateTime(Date.parse(tier.resetsAt))
                            : undefined
                        }
                      >
                        <Clock3 className="size-3" />
                        {resetText(tier, now)}
                      </span>
                    ) : (
                      <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                        {pct(100 - value)}% left
                      </span>
                    )}
                  </div>
                  <div
                    className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted"
                    role="progressbar"
                    aria-label={`${tierLabel(tier)} ${weekly ? "left" : "used"}`}
                    style={
                      weekly
                        ? {
                            background: `color-mix(in srgb, ${color} 12%, var(--muted))`,
                          }
                        : undefined
                    }
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={value}
                  >
                    <div
                      className="h-full rounded-full transition-[width,background-color]"
                      style={{
                        width: `${value}%`,
                        background: color,
                      }}
                    />
                  </div>
                  {!weekly && (
                    <p
                      className="mt-2.5 flex items-center gap-1.5 text-[11px] text-muted-foreground"
                      title={
                        tier.resetsAt
                          ? dateTime(Date.parse(tier.resetsAt))
                          : undefined
                      }
                    >
                      <Clock3 className="size-3" />
                      {resetText(tier, now)}
                    </p>
                  )}
                </div>
              );
            })
          ) : (
            <p className="py-2 text-sm text-muted-foreground">
              {!latest
                ? "The first sample is on its way."
                : latest.status === "ok"
                  ? "No quota windows reported for this account."
                  : "Current quota unavailable. Previous samples remain below."}
            </p>
          )}
        </div>

        <div className="border-t border-border/70 px-5 pt-5 sm:px-7">
          <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="text-xs font-medium">Usage over time</h3>
              <p className="mt-1 text-[11px] text-muted-foreground">
                Last {hours} hours <span className="px-1 text-border">/</span>{" "}
                Used quota (%)
              </p>
            </div>
            <div
              className="flex rounded-lg border border-border/60 bg-muted/60 p-0.5"
              role="group"
              aria-label="Chart time range"
            >
              {[3, 6, 12, 24, 48].map((h) => (
                <Button
                  key={h}
                  size="xs"
                  variant="ghost"
                  aria-pressed={hours === h}
                  className={`h-7 rounded-md px-2.5 text-[11px] sm:px-3 ${hours === h ? "bg-card text-foreground shadow-sm hover:bg-card" : "text-muted-foreground"}`}
                  onClick={() => setHours(h)}
                >
                  {h}h
                </Button>
              ))}
            </div>
          </div>
          <div
            className="relative"
            data-testid="chart-region"
            data-range-start={start}
            data-range-end={end}
          >
            <ChartContainer
              config={config}
              className="h-[220px] w-full aspect-auto sm:h-[240px]"
              aria-label={`${endpoint.name} quota history`}
            >
              <AreaChart
                data={visiblePoints}
                margin={{ top: 10, right: 8, left: -17, bottom: 0 }}
                accessibilityLayer
              >
                <defs>
                  {series.map((s) => (
                    <linearGradient
                      key={s.key}
                      id={`${id}-${s.key}`}
                      x1="0"
                      y1="0"
                      x2="0"
                      y2="1"
                    >
                      <stop
                        offset="0%"
                        stopColor={s.color}
                        stopOpacity={0.18}
                      />
                      <stop
                        offset="100%"
                        stopColor={s.color}
                        stopOpacity={0.01}
                      />
                    </linearGradient>
                  ))}
                </defs>
                <CartesianGrid vertical={false} strokeDasharray="3 5" />
                <XAxis
                  type="number"
                  dataKey="time"
                  domain={[start, end]}
                  allowDataOverflow
                  ticks={Array.from(
                    { length: 5 },
                    (_, i) => start + ((end - start) * i) / 4,
                  )}
                  tickFormatter={(at) =>
                    end - start > 24 * HOUR
                      ? new Date(at).toLocaleString([], {
                          month: "short",
                          day: "numeric",
                          hour: "2-digit",
                          hour12: false,
                        })
                      : clock(at)
                  }
                  axisLine={false}
                  tickLine={false}
                  minTickGap={30}
                  tickMargin={12}
                  fontSize={10}
                />
                <YAxis
                  domain={[0, 100]}
                  ticks={[0, 25, 50, 75, 100]}
                  axisLine={false}
                  tickLine={false}
                  tickMargin={8}
                  fontSize={10}
                  tickFormatter={(v) => `${v}`}
                />
                <ChartTooltip
                  content={
                    <ChartTooltipContent
                      labelFormatter={(_value, payload) =>
                        dateTime(payload[0]?.payload?.time)
                      }
                      formatter={(value, name) => (
                        <div className="flex w-full items-center justify-between gap-7">
                          <span className="text-muted-foreground">
                            {config[String(name)]?.label ?? name}
                          </span>
                          <span className="font-mono font-medium">
                            {pct(Number(value))}%
                          </span>
                        </div>
                      )}
                    />
                  }
                />
                {series.map((s) => (
                  <Area
                    key={s.key}
                    type="monotoneX"
                    dataKey={s.key}
                    name={s.key}
                    stroke={s.color}
                    strokeWidth={2.2}
                    fill={`url(#${id}-${s.key})`}
                    connectNulls={false}
                    dot={
                      goodPoints.length <= 2 ? { r: 3, fill: s.color } : false
                    }
                    activeDot={{ r: 4, stroke: "var(--card)", strokeWidth: 2 }}
                    isAnimationActive={false}
                  />
                ))}
              </AreaChart>
            </ChartContainer>
            {goodPoints.length === 0 && (
              <div className="pointer-events-none absolute inset-0 flex items-center justify-center pb-7">
                <div className="rounded-xl border bg-card/95 px-5 py-4 text-center shadow-sm">
                  <Activity className="mx-auto mb-2 size-5 text-primary" />
                  <p className="text-xs font-medium">
                    No samples in this time range
                  </p>
                  <p className="mt-1 text-[11px] text-muted-foreground">
                    New readings appear every {interval / 60_000} minutes.
                  </p>
                </div>
              </div>
            )}
          </div>
          <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2">
            {series.map((s) => (
              <span
                key={s.key}
                className="flex items-center gap-2 text-[11px] text-muted-foreground"
              >
                <span
                  className="h-0.5 w-3 rounded"
                  style={{ background: s.color }}
                />
                {tierLabel(s.tier)}
              </span>
            ))}
          </div>
        </div>
        <div className="mt-5 flex flex-wrap items-center justify-between gap-2 border-t border-border/60 bg-muted/20 px-5 py-3 text-[10px] text-muted-foreground sm:px-7">
          <span className="flex items-center gap-1.5">
            <span className="size-1 rounded-full bg-muted-foreground/50" />
            {endpoint.history.length} samples retained · Gaps indicate resets or
            missing readings
          </span>
          <span className="tabular-nums">
            {latest
              ? `Sampled ${dateTime(latest.updatedAt)}`
              : "Awaiting first sample"}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
