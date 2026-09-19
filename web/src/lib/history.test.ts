import { test } from "node:test";
import assert from "node:assert/strict";
import { chartPoints, HOUR, seriesFor, visibleRange } from "./history.ts";
import type { Sample } from "./types.ts";

const sample = (
  at: number,
  value: number,
  status: Sample["status"] = "ok",
): Sample => ({
  name: "codex",
  updatedAt: at,
  queriedAt: at,
  status,
  tiers: status === "ok" ? [{ name: "weekly", utilization: value }] : [],
});

test("resets, failures and missed samples break curves without fabricating zeroes", () => {
  const history = [
    sample(0, 20),
    sample(300_000, 30),
    sample(600_000, 0),
    sample(900_000, 0, "error"),
    sample(1200_000, 5),
    sample(2400_000, 10),
  ];
  const points = chartPoints(history, seriesFor(history, null), 300_000);
  assert.deepEqual(
    points.map((p) => [p.time, p.s0]),
    [
      [0, 20],
      [300_000, 30],
      [300_001, null],
      [600_000, 0],
      [900_000, null],
      [1200_000, 5],
      [1200_001, null],
      [2400_000, 10],
    ],
  );
});
test("preset ranges end at the current server time", () => {
  const now = 100 * HOUR;
  assert.deepEqual(visibleRange(now, 3), [now - 3 * HOUR, now]);
  assert.deepEqual(visibleRange(now, 48), [now - 48 * HOUR, now]);
});
