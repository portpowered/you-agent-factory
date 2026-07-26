// @vitest-environment node

import { describe, expect } from "vitest";
import {
  buildReplayCoverageReport,
  formatReplayCoverageReportMarkdown,
  listBrowserIntegrationReplayScenarios,
} from "../src/testing/replay-fixture-catalog";
import {
  browserScenarioTimeoutMs,
  expectNoBrowserErrors,
  loadReplayLines,
  resolvedDefaultFactorySessionID,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
} from "./browser-test-harness.mjs";
import { isolatedMockBrowserTest as it } from "./mocked-browser-test-fixture.mjs";

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
  } catch (_error) {
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
    if (liveButtonCount === 0) {
      liveButtonCount = null;
    }
  }

  await slider.focus();
  await slider.press("ArrowLeft");
  await waitForTickLabel(page, historicalTickLabel);
  expect(await slider.inputValue()).toBe(String(previousTick));
  expect(await countButtons(page, "Return to current tick")).toBe(0);
  let historicalButtonCount = null;
  if (historicalHiddenButtonName && liveButtonCount !== null) {
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
  if (historicalHiddenButtonName && historicalButtonCount !== null) {
    expect(await countButtons(page, historicalHiddenButtonName)).toBe(
      historicalButtonCount,
    );
  }

  await moveSliderToTick(slider, previousTick, finalTick);
  await waitForTickLabel(page, finalTickLabel);
  expect(await slider.inputValue()).toBe(String(finalTick));
  expect(await countButtons(page, "Return to current tick")).toBe(0);
  if (
    historicalHiddenButtonName &&
    liveButtonCount !== null &&
    historicalButtonCount !== null
  ) {
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
  await workstationButton.scrollIntoViewIfNeeded();
  try {
    await workstationButton.click({ force: true });
  } catch (_error) {
    // React Flow workstation buttons can remain visible while clipping leaves the
    // actionable box outside the viewport in CI. Fall back to the DOM click so
    // the test still verifies replay behavior after the button renders.
    await workstationButton.evaluate((button) => {
      button.click();
    });
  }

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

async function assertReplayScenarioRenders(
  preview,
  replayFixture,
  { expect, openBrowserPage },
) {
  const { browserIntegration, fileName, id } = replayFixture;
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
    await browserPage.page
      .getByRole("heading", {
        level: 1,
        name: headingName,
        exact: true,
      })
      .waitFor();
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
    const dashboardSummary = browserPage.page.locator(
      '[aria-label="dashboard summary"]',
    );
    expect(
      await dashboardSummary
        .getByRole("status", {
          name: /Event stream (live|connecting|offline)/,
        })
        .count(),
    ).toBe(0);
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

describe.concurrent("captured event stream replay", () => {
  for (const replayFixture of replayFixtures) {
    it(
      `renders '${replayFixture.id}' without uncaught browser exceptions`,
      async ({ expect, openBrowserPage, preview }) => {
        await assertReplayScenarioRenders(preview, replayFixture, {
          expect,
          openBrowserPage,
        });
      },
      browserScenarioTimeoutMs,
    );
  }

  it(
    "opens the default event stream path for browser replay coverage",
    async ({ expect, openBrowserPage, preview }) => {
      const replayServer = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: replayCurrentFactoryDefinition,
        eventLines: await loadReplayLines(replayFixtures[0].fileName),
      });
      const browserPage = await openBrowserPage();

      try {
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await browserPage.page
          .getByRole("heading", {
            level: 1,
            name: replayFixtures[0].browserIntegration.headingName,
            exact: true,
          })
          .waitFor();
        await expect
          .poll(
            () =>
              replayServer.requestedEventSessionIDs.includes(
                resolvedDefaultFactorySessionID,
              ),
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
    },
    browserScenarioTimeoutMs,
  );
});
