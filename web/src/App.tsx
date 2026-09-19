import { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowUpRight,
  Check,
  Clock3,
  Database,
  Gauge,
  RefreshCw,
  TriangleAlert,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { EndpointCard } from "@/components/endpoint-card";
import type { Dashboard } from "@/lib/types";
import { clock } from "@/lib/format";

export default function App() {
  const [data, setData] = useState<Dashboard | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const pending = useRef<AbortController | null>(null);
  const refresh = useCallback(async () => {
    if (pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    const timeout = window.setTimeout(() => controller.abort(), 15_000);
    setLoading(true);
    try {
      const response = await fetch("/api/usage", {
        signal: controller.signal,
        cache: "no-store",
      });
      if (!response.ok)
        throw new Error("Could not load usage data. Please try again.");
      const body = (await response.json()) as Dashboard;
      setData(body);
      setError("");
    } catch (err) {
      if (!controller.signal.aborted)
        setError(
          err instanceof Error
            ? err.message
            : "Unable to connect to the server.",
        );
      else setError("The server took too long to respond. Please try again.");
    } finally {
      window.clearTimeout(timeout);
      if (pending.current === controller) pending.current = null;
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 60_000);
    const onVisible = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh]);
  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const change = () => {
      document.documentElement.classList.toggle("dark", mq.matches);
      document
        .querySelector('meta[name="theme-color"]')
        ?.setAttribute("content", mq.matches ? "#0a0a0a" : "#ffffff");
    };
    change();
    mq.addEventListener("change", change);
    return () => mq.removeEventListener("change", change);
  }, []);
  const endpoints = data?.endpoints ?? [];
  const healthy = endpoints.filter(
    (e) =>
      e.latest?.status === "ok" &&
      data!.serverTime - e.latest.updatedAt <= data!.sampleIntervalMs * 2,
  ).length;

  return (
    <>
      <header className="site-header">
        <div className="mx-auto flex h-[72px] max-w-[1120px] items-center justify-between px-5 sm:px-8">
          <a
            href="/"
            className="flex items-center gap-2.5 text-sm font-semibold tracking-tight"
          >
            <img className="brand-mark" src="/favicon.svg" alt="" />
            Usage Gauge
          </a>
          <div className="flex items-center gap-4">
            <span className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <span
                className={`size-1.5 rounded-full ${error ? "bg-amber-500" : "bg-primary"}`}
              />
              {error ? "Connection interrupted" : "Auto-updating"}
            </span>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-[1120px] px-5 pb-12 pt-10 sm:px-8 sm:pt-12">
        <div className="mb-8 flex flex-wrap items-end justify-between gap-5">
          <div>
            <p className="mb-2.5 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[2px] text-primary">
              <span className="h-px w-5 bg-primary" />
              Usage analytics
            </p>
            <h1 className="text-[30px] font-semibold leading-tight tracking-[-1.2px] sm:text-[36px]">
              A little clarity on your limits.
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              All your quotas. One quiet place to keep an eye on them.
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="gap-2 bg-card text-xs shadow-xs"
            onClick={() => void refresh()}
            disabled={loading}
          >
            <RefreshCw
              className={`size-3.5 ${loading ? "animate-spin" : ""}`}
            />
            Refresh view
          </Button>
        </div>
        <div className="summary-grid mb-8 rounded-xl border border-border/80 bg-card/70">
          <div className="summary-item">
            <span className="summary-label">
              <Gauge className="size-3.5" />
              Endpoints
            </span>
            <span className="summary-value">
              {data ? String(endpoints.length).padStart(2, "0") : "—"}
              <span className="summary-detail">{healthy} connected</span>
            </span>
          </div>
          <div className="summary-item">
            <span className="summary-label">
              <Clock3 className="size-3.5" />
              Sampling interval
            </span>
            <span className="summary-value">
              {data ? data.sampleIntervalMs / 60_000 : 5}
              <span className="summary-detail">minutes</span>
            </span>
          </div>
          <div className="summary-item">
            <span className="summary-label">
              <Database className="size-3.5" />
              History window
            </span>
            <span className="summary-value">
              48<span className="summary-detail">hours retained</span>
            </span>
          </div>
          <div className="summary-item">
            <span className="summary-label">
              <RefreshCw className="size-3.5" />
              Last successful sample
            </span>
            <span className="summary-value text-lg!">
              {data?.lastUpdatedAt ? clock(data.lastUpdatedAt) : "—"}
              <span className="summary-detail">
                {data?.lastUpdatedAt
                  ? new Date(data.lastUpdatedAt).toLocaleDateString([], {
                      month: "short",
                      day: "numeric",
                    })
                  : "Waiting"}
              </span>
            </span>
          </div>
        </div>
        {error && (
          <div
            role="alert"
            className="mb-6 flex items-center gap-2 rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive"
          >
            <TriangleAlert className="size-4 shrink-0" />
            {error}
            {data && " Showing the last loaded data."}
          </div>
        )}
        <div className="mb-4 flex items-center justify-between">
          <h2 className="flex items-center gap-2 text-sm font-medium">
            Your endpoints{" "}
            <span className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
              {endpoints.length}
            </span>
          </h2>
          <span className="flex items-center gap-1 text-[11px] text-muted-foreground">
            <ArrowUpRight className="size-3" />
            Local time
          </span>
        </div>
        <div className="flex flex-col gap-6">
          {!data &&
            !error &&
            [0, 1].map((i) => (
              <Card key={i} className="p-7" aria-label="Loading endpoint">
                <Skeleton className="h-5 w-40" />
                <Skeleton className="mt-6 h-16 w-full" />
                <Skeleton className="mt-5 h-52 w-full" />
              </Card>
            ))}
          {data && !endpoints.length && (
            <Card className="items-center px-6 py-16 text-center">
              <Gauge className="mb-3 size-9 text-primary" />
              <h3 className="font-medium">Your dashboard is ready.</h3>
              <p className="mt-2 max-w-sm text-sm text-muted-foreground">
                Add an endpoint to endpoints.yaml to start collecting your quota
                history.
              </p>
            </Card>
          )}
          {endpoints.map((endpoint) => (
            <EndpointCard
              key={endpoint.name}
              endpoint={endpoint}
              now={data!.serverTime}
              interval={data!.sampleIntervalMs}
            />
          ))}
        </div>
        <footer className="mt-8 flex flex-wrap items-center justify-between gap-3 px-1 text-[11px] text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <Check className="size-3 text-primary" />
            Stored locally. Always in perspective.
          </span>
          <span>
            usage-gauge <span className="px-2 text-border">/</span> Rolling
            48-hour history
          </span>
        </footer>
      </main>
    </>
  );
}
