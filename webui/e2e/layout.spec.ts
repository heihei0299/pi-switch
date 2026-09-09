import { expect, test } from "@playwright/test";

const state = {
  current: "native",
  profiles: {
    native: {
      api: "openai-completions",
      responsesMode: "passthrough",
      baseUrl: "https://example.test/v1",
      apiKey: "test-key",
      proxy: false,
      headers: {},
      compat: {},
      upstreams: [],
    },
  },
  settings: {
    providerPrefix: "pi-switch",
    writeMode: "gateway",
    gatewayApi: "openai-completions",
    conversationSource: "sessionScan",
    injectOpenCodeAttribution: true,
    proxy: {
      host: "127.0.0.1",
      port: 43112,
      failover: [],
      circuitBreaker: { enabled: true, failureThreshold: 3, cooldownSeconds: 60 },
    },
    web: { host: "127.0.0.1", port: 43110 },
    language: "en",
  },
};

async function mockApi(page: import("@playwright/test").Page) {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/state") return route.fulfill({ json: state });
    if (path === "/api/presets") return route.fulfill({ json: [] });
    if (path === "/api/proxy/status") return route.fulfill({ json: { running: false, message: "stopped" } });
    if (path === "/api/buildInfo") return route.fulfill({
      json: { version: "20260908.0.2", buildTime: "2026-09-10T00:00:00Z", commit: "abc123", target: "linux/amd64", dirty: "false", webui: { embedded: true } },
    });
    if (path === "/api/webui/info") return route.fulfill({ json: { authRequired: false } });
    if (path === "/api/packages") return route.fulfill({ json: { packages: [] } });
    if (path === "/api/backups") return route.fulfill({ json: [] });
    if (path === "/api/doctor" || path === "/api/config/validate") return route.fulfill({ json: [] });
    if (path === "/api/stats") {
      return route.fulfill({
        json: {
          totalRequests: 0,
          okRequests: 0,
          failedRequests: 0,
          successRate: "0%",
          avgLatencyMs: 0,
          byProvider: {},
          byModel: {},
          totalTokens: { input: 0, output: 0, total: 0, cached: 0, reasoning: 0 },
          cacheHitRate: "-",
          totalCost: null,
          costUnknown: 0,
          byConversation: [],
          recentRequests: [],
          recentRequestTotal: 0,
          rows: [],
        },
      });
    }
    if (path === "/api/models/gateway/preview") {
      return route.fulfill({
        json: {
          current: {},
          proposed: {},
          conflicts: [],
          pending_count: 0,
          diff: { added: [], removed: [], changed: [] },
          groups: [],
          removed: [],
        },
      });
    }
    return route.fulfill({ json: { ok: true } });
  });
}

async function assertNoHorizontalOverflow(page: import("@playwright/test").Page) {
  const metrics = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.viewport + 1);
}

test.describe("responsive control surface", () => {
  test("desktop profile surface stays within the viewport", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name === "mobile", "desktop baseline runs only in the desktop project");
    await mockApi(page);
    await page.goto("/");
    await expect(page.getByText("Overview")).toBeVisible();
    await page.locator("aside nav button").filter({ hasText: "Profiles" }).click();
    await expect(page.locator("main").getByText("Profiles").first()).toBeVisible();
    await assertNoHorizontalOverflow(page);

    const image = await page.screenshot({ fullPage: true });
    await testInfo.attach("profiles-desktop-baseline.png", { body: image, contentType: "image/png" });
    expect(image.byteLength).toBeGreaterThan(500);
  });

  test("mobile navigation and profile controls remain reachable", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await mockApi(page);
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Open navigation" })).toBeVisible();
    await page.getByRole("button", { name: "Open navigation" }).click();
    const profilesButton = page.locator("aside nav button").filter({ hasText: "Profiles" });
    await expect(profilesButton).toBeVisible();
    await profilesButton.click();
    await expect(page.locator("main").getByText("Profiles").first()).toBeVisible();
    await assertNoHorizontalOverflow(page);
    await expect(page.getByRole("button", { name: /Add profile/i })).toBeVisible();
  });

  test("settings shows the binary build identity", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name === "mobile", "identity surface is exercised by desktop Chromium");
    await mockApi(page);
    await page.goto("/");
    await page.locator("aside nav button").filter({ hasText: "Settings" }).click();
    await expect(page.getByText("Build identity")).toBeVisible();
    await expect(page.getByText("abc123")).toBeVisible();
    await expect(page.getByText("linux/amd64")).toBeVisible();
  });

  test("viewport matrix has no page overflow", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name === "mobile", "matrix is exercised by the desktop Chromium project");
    await mockApi(page);
    for (const viewport of [
      [375, 812],
      [768, 900],
      [900, 900],
      [1024, 900],
      [1440, 1000],
    ] as const) {
      const [width, height] = viewport;
      await page.setViewportSize({ width, height });
      await page.goto("/");
      if (width < 768) await page.getByRole("button", { name: "Open navigation" }).click();
      await page.locator("aside nav button").filter({ hasText: "Profiles" }).click();
      await expect(page.locator("main").getByText("Profiles").first()).toBeVisible();
      await assertNoHorizontalOverflow(page);
      const image = await page.screenshot({ fullPage: true });
      await testInfo.attach(`viewport-${width}.png`, { body: image, contentType: "image/png" });
      expect(image.byteLength).toBeGreaterThan(500);
    }
  });
});
