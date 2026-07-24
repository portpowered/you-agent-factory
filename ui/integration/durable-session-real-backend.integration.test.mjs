// @vitest-environment node

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  expectNoBrowserErrors,
  gotoDashboardAndWaitForWidgetPicker,
  openBrowserPage,
  readyTimeoutMs,
  selectComboboxOption,
  startBrowserPreview,
  startRealBackendBrowserHarness,
  stopBrowserPreview,
  uiInteractionTimeoutMs,
  waitForDashboardSyncPreflight,
  waitForDashboardWidgetPicker,
} from "./browser-test-harness.mjs";

// Keep this suite bounded to one real-backend proof of the existing durable
// session-detail experience. Fast fixture-backed panel tests and Storybook
// coverage remain the default regression surface for broader UI permutations.
// Focused rerun: `make test-ui-durable-session-real-backend` or
// `cd ui && bun run test:integration:durable-session-real-backend`.

async function openFactorySessionWidget(
  page,
  { widgetPickerTimeoutMs = uiInteractionTimeoutMs } = {},
) {
  await waitForDashboardWidgetPicker(page, widgetPickerTimeoutMs);
  await selectComboboxOption(
    page.getByRole("combobox", { name: "Browse widgets" }),
    "Factory session",
  );
  await page
    .getByRole("button", { name: "Add widget: Factory session" })
    .click();
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
    const syncPreflightResponse = waitForDashboardSyncPreflight(
      browserPage.page,
      readyTimeoutMs,
    );
    await browserPage.page.goto(
      `${preview.previewURL}?factorySessionId=${encodeURIComponent(backend.sessionID)}`,
      { waitUntil: "domcontentloaded" },
    );
    await syncPreflightResponse;
    await openFactorySessionWidget(browserPage.page, {
      widgetPickerTimeoutMs: readyTimeoutMs,
    });
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

// Story 004 disclosure: artifact detail for `child-artifact-1` from the sync
// `agent-run-fake-child` durable session. Broader replay-resume, provider
// transcript, host-matrix, and lifecycle-control proofs stay deferred.
async function assertArtifactDisclosureScenario(preview) {
  const backend = await startRealBackendBrowserHarness({
    apiPort: preview.apiPort,
    startMode: "sync",
    workflowFixture: "agent-run-fake-child.workflow.js",
    workflowName: "agent-run-fake-child",
  });
  const browserPage = await openBrowserPage({
    artifactLabel: "durable-session-real-backend-artifact",
  });
  const artifactDetailPath = `**/factory-sessions/${encodeURIComponent(backend.sessionID)}/artifacts/child-artifact-1`;

  try {
    await browserPage.page.route(
      artifactDetailPath,
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
      .getByRole("heading", { name: "Factory session runtime" })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText(backend.sessionID, { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByRole("button", { name: "Expand dispatch detail for dispatch-1" })
      .click();
    await browserPage.page
      .getByText("Dispatch detail", { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("Dispatch artifacts", { exact: true })
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
    await browserPage.page
      .locator("body")
      .getByText('"dispatchId":"dispatch-1"')
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

async function assertDispatchDetailScenario(preview) {
  const backend = await startRealBackendBrowserHarness({
    apiPort: preview.apiPort,
    startMode: "sync",
    workflowFixture: "agent-run-fake-child.workflow.js",
    workflowName: "agent-run-fake-child",
  });
  const browserPage = await openBrowserPage({
    artifactLabel: "durable-session-real-backend-dispatch",
  });
  const dispatchDetailPath = `**/factory-sessions/${encodeURIComponent(backend.sessionID)}/dispatches/dispatch-1`;

  try {
    await browserPage.page.route(
      dispatchDetailPath,
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
      .getByRole("heading", { name: "Factory session runtime" })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText(backend.sessionID, { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByRole("button", { name: "Expand dispatch detail for dispatch-1" })
      .click();
    await browserPage.page
      .getByText("Loading dispatch detail…", { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByRole("heading", { name: "Factory session runtime" })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("Dispatch detail", { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("JavaScript task", { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("summarize-findings", { exact: true })
      .first()
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("COMPLETED", { exact: true })
      .first()
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("JAVASCRIPT_AGENT", { exact: true })
      .first()
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("fake", { exact: true })
      .first()
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("QUEUED", { exact: true })
      .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
    await browserPage.page
      .getByText("RUNNING", { exact: true })
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
    "shows real durable dispatch detail with loading and terminal backend data through the dashboard factory-session detail path",
    async () => assertDispatchDetailScenario(preview),
    browserScenarioTimeoutMs,
  );

  it(
    "shows real durable artifact detail disclosure from dispatch inspection through the dashboard factory-session detail path",
    async () => assertArtifactDisclosureScenario(preview),
    browserScenarioTimeoutMs,
  );
});
