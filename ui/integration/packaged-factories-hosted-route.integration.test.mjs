// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  findAvailablePort,
  openBrowserPage,
  startBrowserPreview,
  startRealBackendBrowserHarness,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

const viewports = [
  { height: 844, width: 390 },
  { height: 1024, width: 768 },
  { height: 900, width: 1440 },
];

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the sequential browser scenarios share one real-backend lifecycle so the route, keyboard, responsive, and recovery assertions run against the same harness.
describe.sequential("Packaged Factories production route", () => {
  let backend = null;
  let preview = null;

  beforeAll(async () => {
    const apiPort = await findAvailablePort();
    backend = await startRealBackendBrowserHarness({
      apiPort,
      requestID: `req-packaged-factories-${Date.now()}`,
      workflowFixture: "agent-run-fake-child.workflow.js",
      workflowName: "packaged-factories-catalog",
    });
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    await backend?.stop();
    preview = null;
    backend = null;
  });

  it(
    "renders the catalog without page overflow at the hosted dashboard path",
    async () => {
      for (const viewport of viewports) {
        const browserPage = await openBrowserPage({
          artifactLabel: `packaged-factories-${viewport.width}x${viewport.height}`,
        });

        try {
          await browserPage.page.setViewportSize(viewport);
          // The shared preview deliberately owns a separate API port. Bridge
          // this browser scenario to the real backend harness explicitly.
          await browserPage.page.route(
            "**/packaged-factories",
            async (route) => {
              if (
                new URL(route.request().url()).pathname !==
                "/packaged-factories"
              ) {
                await route.continue();
                return;
              }
              const response = await route.fetch({
                url: `${backend.apiOrigin}/packaged-factories`,
              });
              await route.fulfill({ response });
            },
          );
          await browserPage.page.goto(
            new URL("packaged-factories", preview.previewURL).href,
            { waitUntil: "domcontentloaded" },
          );
          await browserPage.page
            .getByRole("heading", { level: 2, name: "Packaged Factories" })
            .waitFor({
              state: "visible",
              timeout: uiInteractionTimeoutMs,
            });
          await browserPage.page
            .getByText(
              "Breaks a research question into bounded specialist investigations and synthesizes their findings.",
              { exact: true },
            )
            .waitFor({
              state: "visible",
              timeout: uiInteractionTimeoutMs,
            });

          const inventory = browserPage.page.getByRole("navigation", {
            name: "Available Packaged Factories",
          });
          const factoryButtons = inventory.getByRole("button");
          const firstFactory = factoryButtons.first();
          const secondFactory = factoryButtons.nth(1);
          await firstFactory.focus();
          await browserPage.page.keyboard.press("ArrowDown");
          await browserPage.page.keyboard.press("Enter");
          await secondFactory.waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });
          expect(await secondFactory.getAttribute("aria-current")).toBe("true");
          await browserPage.page.getByRole("heading", { level: 3 }).waitFor({
            state: "visible",
            timeout: uiInteractionTimeoutMs,
          });

          expect(
            await browserPage.page.evaluate(
              () =>
                document.documentElement.scrollWidth <=
                document.documentElement.clientWidth,
            ),
          ).toBe(true);
          expectNoBrowserErrors(
            browserPage.pageErrors,
            browserPage.consoleErrors,
            expect,
          );
        } finally {
          await browserPage.close();
        }
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "presents a recoverable catalog error and reloads through the API boundary",
    async () => {
      const browserPage = await openBrowserPage({
        artifactLabel: "packaged-factories-error-recovery",
      });

      try {
        let requestCount = 0;
        await browserPage.page.route("**/packaged-factories", async (route) => {
          if (
            new URL(route.request().url()).pathname !== "/packaged-factories"
          ) {
            await route.continue();
            return;
          }
          requestCount += 1;
          if (requestCount === 1) {
            await route.fulfill({
              body: JSON.stringify({ code: "INTERNAL_ERROR" }),
              contentType: "application/json",
              status: 500,
            });
            return;
          }
          const response = await route.fetch({
            url: `${backend.apiOrigin}/packaged-factories`,
          });
          await route.fulfill({ response });
        });
        await browserPage.page.goto(
          new URL("packaged-factories", preview.previewURL).href,
          { waitUntil: "domcontentloaded" },
        );

        await browserPage.page
          .getByRole("alert")
          .filter({ hasText: "The Packaged Factory catalog is unavailable." })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page.getByRole("button", { name: "Retry" }).click();
        await browserPage.page
          .getByRole("heading", { level: 3 })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        expect(requestCount).toBe(2);
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
      }
    },
    browserScenarioTimeoutMs,
  );
});
