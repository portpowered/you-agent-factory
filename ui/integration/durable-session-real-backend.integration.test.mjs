// @vitest-environment node

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  gotoDashboardAndWaitForWidgetPicker,
  openBrowserPage,
  selectComboboxOption,
  startBrowserPreview,
  stopBrowserPreview,
  startRealBackendBrowserHarness,
  uiInteractionTimeoutMs,
  waitForDashboardWidgetPicker,
} from "./browser-test-harness.mjs";

// Keep this suite bounded to one real-backend proof of the existing durable
// session-detail experience. Fast fixture-backed panel tests and Storybook
// coverage remain the default regression surface for broader UI permutations.

async function openFactorySessionWidget(page) {
  const browseWidgets = await waitForDashboardWidgetPicker(page);
  await selectComboboxOption(browseWidgets, "Factory session");
  await page.getByRole("button", { name: "Add widget: Factory session" }).click();
  await page
    .getByRole("heading", {
      exact: true,
      level: 3,
      name: "Factory session",
    })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
}

async function assertRunningSummaryScenario(preview) {
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
    await gotoDashboardAndWaitForWidgetPicker(
      browserPage.page,
      `${preview.previewURL}?factorySessionId=${encodeURIComponent(backend.sessionID)}`,
    );
    await openFactorySessionWidget(browserPage.page);
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
}

async function assertDispatchArtifactScenario(preview) {
  const backend = await startRealBackendBrowserHarness({
    apiPort: preview.apiPort,
    startMode: "sync",
    workflowFixture: "agent-run-fake-child.workflow.js",
    workflowName: "agent-run-fake-child",
  });
  const browserPage = await openBrowserPage({
    artifactLabel: "durable-session-real-backend-dispatch-artifact",
  });

  try {
    await gotoDashboardAndWaitForWidgetPicker(
      browserPage.page,
      `${preview.previewURL}?factorySessionId=${encodeURIComponent(backend.sessionID)}`,
    );
    await openFactorySessionWidget(browserPage.page);
    await browserPage.page
      .getByRole("heading", { name: "Factory session runtime" })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText(backend.sessionID, { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByRole("button", { name: "Expand dispatch detail for dispatch-1" })
      .click();
    await browserPage.page
      .getByText("JavaScript task", { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("summarize-findings", { exact: true })
      .first()
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("fake", { exact: true })
      .first()
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    const artifactLink = browserPage.page.getByRole("link", {
      name: "child-artifact-1",
    });
    await artifactLink.waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
    await Promise.all([
      browserPage.page.waitForURL(
        new RegExp(
          `/factory-sessions/${encodeURIComponent(backend.sessionID)}/artifacts/child-artifact-1$`,
        ),
        { timeout: uiInteractionTimeoutMs },
      ),
      artifactLink.click(),
    ]);
    await browserPage.page
      .locator("body")
      .getByText('"id":"child-artifact-1"')
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });

    expectNoBrowserErrors(
      browserPage.pageErrors,
      browserPage.consoleErrors,
      expect,
    );
  } finally {
    await browserPage.close();
    await backend.stop();
  }
}

describe.sequential("durable session real backend browser integration", () => {
  let preview = null;

  beforeEach(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterEach(async () => {
    await stopBrowserPreview();
    preview = null;
  });

  it(
    "shows loading and real durable summary state for a running backend session through the dashboard factory-session detail path",
    async () => assertRunningSummaryScenario(preview),
    browserScenarioTimeoutMs,
  );

  it(
    "shows real durable dispatch detail and artifact drilldown for a completed backend session through the dashboard factory-session detail path",
    async () => assertDispatchArtifactScenario(preview),
    browserScenarioTimeoutMs,
  );
});
