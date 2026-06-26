// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  openBrowserPage,
  selectComboboxOption,
  startBrowserPreview,
  startRealBackendBrowserHarness,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";

describe.sequential("durable session real backend browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "shows loading and real durable summary state for a running backend session through the dashboard factory-session detail path",
    async () => {
      const backend = await startRealBackendBrowserHarness({
        apiPort: preview.apiPort,
        startMode: "async",
        workflowFixture: "busy-loop.workflow.js",
        workflowName: "busy-loop",
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "durable-session-real-backend",
      });

      try {
        await browserPage.page.route(
          `**/factory-sessions/${encodeURIComponent(backend.sessionID)}`,
          async (route) => {
            await new Promise((resolve) => setTimeout(resolve, 750));
            await route.continue();
          },
          { times: 1 },
        );
        await browserPage.page.goto(
          `${preview.previewURL}?factorySessionId=${encodeURIComponent(backend.sessionID)}`,
          {
            waitUntil: "domcontentloaded",
          },
        );
        await browserPage.page
          .getByRole("heading", { level: 1, name: "U", exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await selectComboboxOption(
          browserPage.page.getByRole("combobox", { name: "Browse widgets" }),
          "Factory session",
        );
        await browserPage.page
          .getByRole("button", { name: "Add widget: Factory session" })
          .click();
        await browserPage.page
          .getByRole("heading", {
            exact: true,
            level: 3,
            name: "Factory session",
          })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("status")
          .filter({ hasText: "Loading factory session runtime…" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByRole("heading", { name: "Factory session runtime" })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByText(backend.sessionID, { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByText("JavaScript workflow", { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByText("Running", { exact: true })
          .first()
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        expect(
          await browserPage.page.getByText("Running", { exact: true }).count(),
        ).toBeGreaterThanOrEqual(2);
        await browserPage.page
          .getByText("Child dispatches", { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
        await browserPage.page
          .getByText("queued 0, running 0, completed 0", { exact: true })
          .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

        expect(backend.sessionID.startsWith("dur-sess-")).toBe(true);
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await backend.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
