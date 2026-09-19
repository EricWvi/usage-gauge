import { test, expect } from "@playwright/test";

function fixture() {
  const now = Date.now();
  return {
    serverTime: now,
    lastUpdatedAt: now,
    sampleIntervalMs: 300000,
    retentionHours: 48,
    endpoints: ["codex", "zai"].map((name, n) => {
      const history = Array.from({ length: 577 }, (_, i) => {
        const at = now - (576 - i) * 300000;
        const tiers = [
          {
            name: name === "codex" ? "codex:primary" : "five_hour",
            label: "five_hour",
            utilization: 10 + (i % 60) * 0.93 + Math.sin(i / 4) * 1.6,
            resetsAt: new Date(now + 2.5 * 3600000).toISOString(),
          },
        ];
        if (!n)
          tiers.push({
            name: "codex:secondary",
            label: "weekly",
            utilization: 14 + i * 0.074 + Math.sin(i / 20) * 0.5,
            resetsAt: new Date(now + 3.2 * 86400000).toISOString(),
          });
        return {
          name,
          updatedAt: at,
          queriedAt: at,
          status: "ok",
          message: n ? "GLM Coding Plan Pro" : "plus",
          tiers,
        };
      });
      return { name, provider: name, history, latest: history.at(-1) };
    }),
  };
}

test("chart presets, polling, and system themes", async ({ page }, info) => {
  let requests = 0;
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const data = fixture();
  await page.route("**/api/usage", (route) => {
    requests++;
    return route.fulfill({
      json: { ...data, serverTime: data.serverTime + requests * 1000 },
    });
  });
  await page.clock.install();
  await page.emulateMedia({ colorScheme: "light" });
  await page.addInitScript(() => localStorage.setItem("theme", "dark"));
  await page.goto("/");
  await expect(page.locator("html")).not.toHaveClass("dark");
  await expect(
    page.getByRole("button", { name: /Switch to .* theme/ }),
  ).toHaveCount(0);
  const codex = page.locator('[data-endpoint="codex"]');
  await expect(
    codex.getByRole("heading", { name: "codex", exact: true }),
  ).toBeVisible();
  await expect(codex.locator(".recharts-area-curve").first()).toBeVisible();
  const chartSurface = codex.locator(".recharts-wrapper");
  await chartSurface.hover({ position: { x: 300, y: 120 } });
  await expect(codex).not.toContainText("Invalid Date");
  await expect(
    codex.getByText("Weekly window", { exact: true }).first(),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Usage Gauge" })).toBeVisible();
  await expect(page.locator('link[rel="icon"]')).toHaveAttribute(
    "href",
    "/favicon.svg",
  );
  const chart = codex.getByTestId("chart-region");
  const duration = async () =>
    Number(await chart.getAttribute("data-range-end")) -
    Number(await chart.getAttribute("data-range-start"));
  expect(await duration()).toBe(24 * 3600000);
  await page.screenshot({
    path: info.outputPath("dashboard-light.png"),
    fullPage: true,
  });
  await codex.getByRole("button", { name: "3h", exact: true }).click();
  expect(await duration()).toBe(3 * 3600000);
  await page.clock.fastForward(61000);
  await expect.poll(() => requests).toBeGreaterThan(1);
  expect(await duration()).toBe(3 * 3600000);
  await codex.getByRole("button", { name: "48h", exact: true }).click();
  expect(await duration()).toBe(48 * 3600000);
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator("html")).toHaveClass("dark");
  await page.screenshot({
    path: info.outputPath("dashboard-dark.png"),
    fullPage: true,
  });
  await page.emulateMedia({ colorScheme: "light" });
  await expect(page.locator("html")).not.toHaveClass("dark");
  const beforeVisible = requests;
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange")),
  );
  await expect.poll(() => requests).toBeGreaterThan(beforeVisible);
  expect(errors).toEqual([]);
});

test("mobile layout and error samples", async ({ page }, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const data = fixture();
  data.endpoints[1].latest = {
    ...data.endpoints[1].latest!,
    status: "error",
    tiers: [],
  };
  await page.route("**/api/usage", (route) => route.fulfill({ json: data }));
  await page.goto("/");
  await expect(page.getByText("Query failed", { exact: true })).toBeVisible();
  await expect(page.locator(".recharts-area-curve").first()).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("dashboard-mobile.png"),
    fullPage: true,
  });
});

test("empty configuration, pending sampling, and server failure", async ({
  page,
}) => {
  let body: object = { ...fixture(), endpoints: [] };
  let status = 200;
  await page.route("**/api/usage", (route) =>
    route.fulfill({ status, json: body }),
  );
  await page.goto("/");
  await expect(page.getByText("Your dashboard is ready.")).toBeVisible();
  body = {
    ...fixture(),
    endpoints: [
      { name: "codex", provider: "codex", latest: null, history: [] },
    ],
  };
  await page.getByRole("button", { name: "Refresh view" }).click();
  await expect(page.getByText("The first sample is on its way.")).toBeVisible();
  status = 500;
  await page.getByRole("button", { name: "Refresh view" }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Showing the last loaded data",
  );
  await expect(page.getByText("The first sample is on its way.")).toBeVisible();
});

test("weekly quota shows remaining capacity and shifts from green to red", async ({
  page,
}) => {
  const data = fixture();
  const weekly = data.endpoints[0].latest!.tiers[1];
  weekly.utilization = 0;
  await page.route("**/api/usage", (route) => route.fulfill({ json: data }));
  await page.goto("/");
  const metric = page.locator('[data-tier="codex:secondary"]');
  const bar = metric.getByRole("progressbar", { name: "Weekly window left" });
  const fill = bar.locator("div");
  const primary = metric.getByTestId("quota-value");
  await expect(bar).toHaveAttribute("aria-valuenow", "100");
  await expect(primary).toHaveText("100%");
  await expect(fill).toHaveCSS(
    "width",
    `${await bar.evaluate((el) => el.getBoundingClientRect().width)}px`,
  );
  const fullColor = await fill.evaluate(
    (el) => (el as HTMLElement).style.backgroundColor,
  );
  await expect(metric.getByText(/Resets in 3d/)).toBeVisible();
  await expect(metric.getByText(/% used$/)).toHaveCount(0);
  weekly.utilization = 75;
  await page.getByRole("button", { name: "Refresh view" }).click();
  await expect(bar).toHaveAttribute("aria-valuenow", "25");
  await expect(primary).toHaveText("25%");
  await expect(metric.getByText(/% used$/)).toHaveCount(0);
  const lowColor = await fill.evaluate(
    (el) => (el as HTMLElement).style.backgroundColor,
  );
  expect(lowColor).not.toBe(fullColor);
  weekly.utilization = 100;
  await page.getByRole("button", { name: "Refresh view" }).click();
  await expect(bar).toHaveAttribute("aria-valuenow", "0");
  await expect(primary).toHaveText("0%");
  await expect(fill).toHaveCSS("width", "0px");
  const emptyColor = await fill.evaluate(
    (el) => (el as HTMLElement).style.backgroundColor,
  );
  expect(emptyColor).not.toBe(fullColor);
  await expect(primary).toHaveCSS("color", emptyColor);
  await expect(
    page.getByRole("progressbar", { name: "5-hour window used" }).first(),
  ).toBeVisible();
});
