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
          // The Vite preview serves the dashboard only. Proxy this request to
          // the real backend so the rendered description proves the complete
          // catalog handler-to-browser contract.
          await browserPage.page.route("**/packaged-factories", async (route) => {
            if (new URL(route.request().url()).pathname !== "/packaged-factories") {
              return route.continue();
            }
            const response = await route.fetch({
              url: `${backend.apiOrigin}/packaged-factories`,
            });
            return route.fulfill({ response });
          });
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
});
