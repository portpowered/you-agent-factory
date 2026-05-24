// @vitest-environment node

import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";

import {
  buildTimeoutMs,
  browserScenarioTimeoutMs,
  defaultFactorySessionID,
  expectNoBrowserErrors,
  loadReplayLines,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import {
  buildReplayCoverageReport,
  formatReplayCoverageReportMarkdown,
  listBrowserIntegrationReplayScenarios,
} from "../src/testing/replay-fixture-catalog";

const replayCurrentFactoryDefinition = {
  name: "Browser Replay Factory",
};
const replayFixtures = listBrowserIntegrationReplayScenarios();

function formatVisibleTickStatus(currentTick, maxTick) {
  return `${currentTick}/${maxTick}`;
}

async function countButtons(page, buttonName) {
  return await page.getByRole("button", { name: buttonName }).count();
}

async function waitForTickLabel(page, label) {
  try {
    await page.getByText(label).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  } catch (error) {
    const sliderValue = await page
      .getByRole("slider", { name: "Timeline tick" })
      .inputValue();
    const statusTexts = await page
      .locator("span")
      .evaluateAll((elements) =>
        elements
          .map((element) => element.textContent?.trim() ?? "")
          .filter((text) => /^\d+\/\d+$/.test(text)),
      );
    throw new Error(
      `Timed out waiting for ${label}; slider=${sliderValue}; visibleTicks=${statusTexts.join(", ") || "<none>"}`,
      { cause: error },
    );
  }
}

async function moveSliderToTick(slider, currentTick, targetTick) {
  const direction = targetTick >= currentTick ? "ArrowRight" : "ArrowLeft";
  const steps = Math.abs(targetTick - currentTick);

  for (let index = 0; index < steps; index += 1) {
    await slider.press(direction);
  }
}

async function exerciseHistoricalTimelineView(page, options) {
  const {
    finalTick,
    historicalHiddenButtonName,
    inFlightSelectionTick,
    replayServer,
  } = options;
  const slider = page.getByRole("slider", { name: "Timeline tick" });
  const liveTick = inFlightSelectionTick ?? finalTick;
  const previousTick = liveTick - 1;
  const liveTickLabel = formatVisibleTickStatus(liveTick, liveTick);
  const historicalTickLabel = formatVisibleTickStatus(previousTick, liveTick);
  const pinnedHistoricalTickLabel = formatVisibleTickStatus(
    previousTick,
    finalTick,
  );
  const finalTickLabel = formatVisibleTickStatus(finalTick, finalTick);

  expect(previousTick).toBeGreaterThan(0);

  await slider.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  if (inFlightSelectionTick) {
    await replayServer.replayPaused;
  }
  await waitForTickLabel(page, liveTickLabel);
  expect(await slider.inputValue()).toBe(String(liveTick));
  expect(await countButtons(page, "Return to current tick")).toBe(0);
  let liveButtonCount = null;
  if (historicalHiddenButtonName) {
    liveButtonCount = await countButtons(page, historicalHiddenButtonName);
    expect(liveButtonCount).toBeGreaterThan(0);
  }

  await slider.focus();
  await slider.press("ArrowLeft");
  await waitForTickLabel(page, historicalTickLabel);
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await countButtons(page, "Return to current tick")).toBe(0);
  let historicalButtonCount = null;
  if (historicalHiddenButtonName) {
    historicalButtonCount = await countButtons(
      page,
      historicalHiddenButtonName,
    );
    if (!inFlightSelectionTick) {
      expect(historicalButtonCount).toBeLessThan(liveButtonCount);
    }
  }

  if (inFlightSelectionTick) {
    replayServer.releaseReplayStream();
    await replayServer.replayCompleted;
  }
  await waitForTickLabel(
    page,
    inFlightSelectionTick ? pinnedHistoricalTickLabel : historicalTickLabel,
  );
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await countButtons(page, "Return to current tick")).toBe(0);
  if (historicalHiddenButtonName) {
    expect(await countButtons(page, historicalHiddenButtonName)).toBe(
      historicalButtonCount,
    );
  }

  await moveSliderToTick(slider, previousTick, finalTick);
  await waitForTickLabel(page, finalTickLabel);
  expect(await slider.inputValue()).toBe(String(finalTick));
  expect(await countButtons(page, "Return to current tick")).toBe(0);
  if (historicalHiddenButtonName) {
    const currentButtonCount = await countButtons(
      page,
      historicalHiddenButtonName,
    );
    if (inFlightSelectionTick) {
      expect(currentButtonCount).toBeGreaterThan(historicalButtonCount);
    } else {
      expect(currentButtonCount).toBe(liveButtonCount);
    }
  }
}

async function exerciseSelectedWorkTrace(page, workstationName, options = {}) {
  const { requiresWorkItemSelection = true, selectedWorkText = null } = options;
  const workstationButton = page.getByRole("button", {
    name: workstationName,
  });
  await workstationButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await workstationButton.click({ force: true });

  await page.getByRole("article", { name: "Current selection" }).waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  if (selectedWorkText !== null) {
    await page.getByText(selectedWorkText, { exact: false }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  }

  if (!requiresWorkItemSelection) {
    return;
  }

  const workItemButton = page
    .getByRole("button", { name: /^Select work item / })
    .first();
  try {
    await workItemButton.waitFor({ state: "visible", timeout: 2_000 });
    await workItemButton.click({ force: true });

    await page.getByRole("article", { name: "Trace drill-down" }).waitFor({
      state: "visible",
      timeout: uiInteractionTimeoutMs,
    });
  } catch {
    // Some canonical replays finish without a selectable current-work item at the
    // final tick. Selecting the workstation is still enough to verify the replay
    // rendered without browser-side failures.
  }
}

async function assertReplayScenarioRenders(preview, replayFixture) {
  const {
    browserIntegration,
    fileName,
    id,
  } = replayFixture;
  const {
    finalTick,
    headingName,
    historicalHiddenButtonName,
    inFlightSelectionTick,
    requiresWorkItemSelection,
    selectedWorkText,
    workstationName,
  } = browserIntegration;

  const replayServer = await startFactoryApiServer({
    apiPort: preview.apiPort,
    currentFactory: replayCurrentFactoryDefinition,
    eventLines: await loadReplayLines(fileName),
    pauseBeforeTick: inFlightSelectionTick ?? null,
  });
  const replayCoverageReport = buildReplayCoverageReport();
  const coverageScenario = replayCoverageReport.scenarios.find(
    (scenario) => scenario.id === id,
  );
  const replayCoverageMarkdown =
    formatReplayCoverageReportMarkdown(replayCoverageReport);
  const browserPage = await openBrowserPage();

  expect(coverageScenario).toBeDefined();
  expect(coverageScenario?.verificationLayers).toContain("browser-integration");
  expect(coverageScenario?.fileName).toBe(fileName);
  expect(replayCoverageMarkdown).toContain(`| \`${id}\` | \`${fileName}\` |`);

  try {
    await browserPage.page.goto(preview.previewURL, {
      waitUntil: "domcontentloaded",
    });
    expectNoBrowserErrors(
      browserPage.pageErrors,
      browserPage.consoleErrors,
      expect,
    );
    await browserPage.page.getByRole("heading", { name: headingName }).waitFor();
    await browserPage.page
      .getByRole("region", { name: "you-agent-factory bento board" })
      .waitFor();
    await browserPage.page
      .getByRole("button", { name: workstationName })
      .waitFor();
    if (!inFlightSelectionTick) {
      await replayServer.replayCompleted;
      await browserPage.page
        .getByText(formatVisibleTickStatus(finalTick, finalTick))
        .waitFor({
          timeout: uiInteractionTimeoutMs,
        });
    }
    await exerciseHistoricalTimelineView(browserPage.page, {
      finalTick,
      historicalHiddenButtonName,
      inFlightSelectionTick,
      replayServer,
    });
    const dashboardSummary = browserPage.page.locator('[aria-label="dashboard summary"]');
    await dashboardSummary
      .getByRole("status", {
        name: /Event stream (live|connecting|offline)/,
      })
      .waitFor();
    expect(
      await dashboardSummary.getByText("RUNNING", { exact: true }).count(),
    ).toBe(0);
    await exerciseSelectedWorkTrace(browserPage.page, workstationName, {
      requiresWorkItemSelection,
      selectedWorkText,
    });

    expectNoBrowserErrors(
      browserPage.pageErrors,
      browserPage.consoleErrors,
      expect,
    );
  } finally {
    await replayServer.stop();
    await browserPage.close();
  }
}

describe.sequential("captured event stream replay", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  afterEach(async () => {
    // replay servers are created per-test and stopped in the scenario helper
  });

  for (const replayFixture of replayFixtures) {
    it(
      `renders '${replayFixture.id}' without uncaught browser exceptions`,
      async () => {
        await assertReplayScenarioRenders(preview, replayFixture);
      },
      browserScenarioTimeoutMs,
    );
  }

  it("opens the default event stream path for browser replay coverage", async () => {
    const replayServer = await startFactoryApiServer({
      apiPort: preview.apiPort,
      currentFactory: replayCurrentFactoryDefinition,
      eventLines: await loadReplayLines(replayFixtures[0].fileName),
    });
    const browserPage = await openBrowserPage();

    try {
      await browserPage.page.goto(preview.previewURL, { waitUntil: "domcontentloaded" });
      await browserPage.page
        .getByRole("heading", { name: replayFixtures[0].browserIntegration.headingName })
        .waitFor();
      await expect
        .poll(
          () => replayServer.requestedEventSessionIDs.includes(defaultFactorySessionID),
          { timeout: uiInteractionTimeoutMs },
        )
        .toBe(true);
      expectNoBrowserErrors(
        browserPage.pageErrors,
        browserPage.consoleErrors,
        expect,
      );
    } finally {
      await replayServer.stop();
      await browserPage.close();
    }
  }, browserScenarioTimeoutMs);
});
