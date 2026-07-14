// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  initialEditableFactoryDefinitionVersion,
  installBrowserErrorCapture,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
  waitForDashboardWidgetPicker,
  waitForDurableCheckpoint,
} from "./browser-test-harness.mjs";
import {
  clearTimelineCheckpoints,
  defaultFactoryDefinition,
  installNetworkCapture,
  seedTimelineCheckpoint,
} from "./dashboard-session-recovery-manual-scenarios-harness.mjs";
import {
  alphaSessionID,
  betaSessionID,
  checkpointFixture,
  deleteCheckpointEnvelope,
  eventStreamURL,
  matchingTailURLs,
  readCheckpointEnvelope,
  sessionFixture,
  streamIdentity,
  tailEvent,
} from "./dashboard-shared-indexeddb-browser-contexts-fixtures.mjs";

async function selectSession(page, name) {
  const tab = page.getByRole("tab", { name });
  await tab.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await tab.click();
}

async function expectRestoredPage(
  page,
  expectedTick,
  expectedChartTicks = String(expectedTick),
) {
  await waitForDurableCheckpoint(
    `restored timeline tick ${expectedTick}`,
    async () =>
      (await page
        .getByRole("slider", { name: "Timeline tick" })
        .inputValue()
        .catch(() => "")) === String(expectedTick),
  );
  await expect
    .poll(
      async () =>
        page
          .locator("[data-work-chart-visible-ticks]")
          .getAttribute("data-work-chart-visible-ticks"),
      { timeout: uiInteractionTimeoutMs },
    )
    .toBe(expectedChartTicks);
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the suite owns one shared preview lifecycle around its two-page scenario.
describe.sequential("shared IndexedDB dashboard browser contexts", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "restores isolated concrete sessions and resumes one tail across lifecycle",
    // biome-ignore lint/complexity/noExcessiveLinesPerFunction: one shared browser context and finally-equivalent teardown make the scenario lifecycle explicit.
    async () => {
      const alphaSession = sessionFixture(alphaSessionID, "alpha");
      const betaSession = sessionFixture(betaSessionID, "beta");
      const alphaIdentity = streamIdentity(
        alphaSessionID,
        "alpha",
        initialEditableFactoryDefinitionVersion.physical,
      );
      const betaIdentity = streamIdentity(
        betaSessionID,
        "beta",
        initialEditableFactoryDefinitionVersion.physical,
      );
      const staleAlphaIdentity = streamIdentity(
        alphaSessionID,
        "alpha",
        "2026-01-01T00:00:00Z",
      );
      const alphaCheckpoint = checkpointFixture({
        eventID: "alpha-restored-17",
        sampleTicks: [2, 3, 4, 5, 6, 7],
        sequence: 17,
        tick: 7,
        value: 6,
      });
      const betaCheckpoint = checkpointFixture({
        eventID: "beta-restored-29",
        sequence: 29,
        tick: 13,
        value: 5,
      });
      let browserPage = null;
      let server = null;
      let tabTwo = null;

      try {
        server = await startFactoryApiServer({
          apiPort: preview.apiPort,
          currentFactory: defaultFactoryDefinition,
          currentFactoryBySessionID: {
            [alphaSessionID]: defaultFactoryDefinition,
            [betaSessionID]: defaultFactoryDefinition,
          },
          eventLines: [
            tailEvent({ eventID: "default-ready-1", sequence: 1, tick: 1 }),
          ],
          eventLinesBySessionID: {
            [alphaSessionID]: [
              tailEvent({
                eventID: alphaCheckpoint.afterEventId,
                sequence: alphaCheckpoint.afterSequence,
                tick: alphaCheckpoint.selectedTick,
              }),
              tailEvent({ eventID: "alpha-live-18", sequence: 18, tick: 21 }),
              tailEvent({ eventID: "alpha-live-18", sequence: 18, tick: 21 }),
            ],
            [betaSessionID]: [
              tailEvent({
                eventID: betaCheckpoint.afterEventId,
                sequence: betaCheckpoint.afterSequence,
                tick: betaCheckpoint.selectedTick,
              }),
            ],
          },
          pauseBeforeTick: 20,
          sessions: [alphaSession, betaSession],
        });
        browserPage = await openBrowserPage({
          artifactLabel: "shared-indexeddb-two-session-tabs",
          artifactMode: "bounded",
        });
        tabTwo = await browserPage.context.newPage();
        const tabTwoErrors = installBrowserErrorCapture(tabTwo, {
          characterLimit: 512,
          entryLimit: 16,
        });
        const alphaNetwork = await installNetworkCapture(browserPage.page);
        const betaNetwork = await installNetworkCapture(tabTwo);
        await Promise.all([
          browserPage.page.goto(preview.previewURL, {
            waitUntil: "domcontentloaded",
          }),
          tabTwo.goto(preview.previewURL, { waitUntil: "domcontentloaded" }),
        ]);
        await Promise.all([
          waitForDashboardWidgetPicker(browserPage.page),
          waitForDashboardWidgetPicker(tabTwo),
        ]);
        expect(new URL(browserPage.page.url()).origin).toBe(
          new URL(tabTwo.url()).origin,
        );

        await clearTimelineCheckpoints(browserPage.page);
        await Promise.all([
          seedTimelineCheckpoint(
            browserPage.page,
            alphaIdentity,
            alphaCheckpoint,
          ),
          seedTimelineCheckpoint(tabTwo, betaIdentity, betaCheckpoint),
        ]);
        await seedTimelineCheckpoint(
          browserPage.page,
          staleAlphaIdentity,
          checkpointFixture({
            eventID: "stale-alpha-99",
            sequence: 99,
            tick: 99,
            value: 99,
          }),
        );
        await deleteCheckpointEnvelope(browserPage.page, staleAlphaIdentity);

        await Promise.all([
          alphaNetwork.resetEventStreamURLs(),
          betaNetwork.resetEventStreamURLs(),
        ]);
        await Promise.all([
          browserPage.page.reload({ waitUntil: "domcontentloaded" }),
          tabTwo.reload({ waitUntil: "domcontentloaded" }),
        ]);
        await Promise.all([
          selectSession(browserPage.page, "alpha"),
          selectSession(tabTwo, "beta"),
        ]);

        await waitForDurableCheckpoint(
          "one scoped event tail per restored session",
          async () => {
            const [alphaURLs, betaURLs] = await Promise.all([
              alphaNetwork.readEventStreamURLs(),
              betaNetwork.readEventStreamURLs(),
            ]);
            return (
              matchingTailURLs(alphaURLs, alphaSessionID).length === 1 &&
              matchingTailURLs(betaURLs, betaSessionID).length === 1
            );
          },
        );
        await Promise.all([
          expectRestoredPage(browserPage.page, 7, "2,3,4,5,6,7"),
          expectRestoredPage(tabTwo, 13),
        ]);

        const restoredAlphaNetworkURLs =
          await alphaNetwork.readEventStreamURLs();
        const restoredBetaNetworkURLs = await betaNetwork.readEventStreamURLs();
        const alphaURLs = matchingTailURLs(
          restoredAlphaNetworkURLs,
          alphaSessionID,
        );
        const betaURLs = matchingTailURLs(
          restoredBetaNetworkURLs,
          betaSessionID,
        );
        expect(alphaURLs).toHaveLength(1);
        expect(betaURLs).toHaveLength(1);
        expect(
          matchingTailURLs(restoredAlphaNetworkURLs, betaSessionID),
        ).toEqual([]);
        expect(
          matchingTailURLs(restoredBetaNetworkURLs, alphaSessionID),
        ).toEqual([]);
        expect(
          eventStreamURL(alphaURLs[0]).searchParams.get("after_event_id"),
        ).toBe(alphaCheckpoint.afterEventId);
        expect(
          eventStreamURL(betaURLs[0]).searchParams.get("after_event_id"),
        ).toBe(betaCheckpoint.afterEventId);
        expect(
          eventStreamURL(alphaURLs[0]).searchParams.get("after_sequence"),
        ).toBe(String(alphaCheckpoint.afterSequence));
        expect(
          eventStreamURL(betaURLs[0]).searchParams.get("after_sequence"),
        ).toBe(String(betaCheckpoint.afterSequence));

        const [durableAlpha, durableBeta] = await Promise.all([
          readCheckpointEnvelope(browserPage.page, alphaIdentity),
          readCheckpointEnvelope(tabTwo, betaIdentity),
        ]);
        expect(
          await readCheckpointEnvelope(browserPage.page, staleAlphaIdentity),
        ).toBeNull();
        expect(durableAlpha.checkpoint.afterEventId).toBe("alpha-restored-17");
        expect(durableBeta.checkpoint.afterEventId).toBe("beta-restored-29");
        expect(durableAlpha.checkpoint.selectedTick).toBe(7);
        expect(durableBeta.checkpoint.selectedTick).toBe(13);

        server.releaseReplayStream();
        await Promise.all([
          expectRestoredPage(browserPage.page, 21, "2,3,4,5,6,7,21"),
          expectRestoredPage(tabTwo, 13),
        ]);
        expect(
          await browserPage.page
            .locator("[data-work-chart-visible-ticks]")
            .getAttribute("data-work-chart-visible-ticks"),
        ).toBe("2,3,4,5,6,7,21");
        expect(
          await tabTwo
            .locator("[data-work-chart-visible-ticks]")
            .getAttribute("data-work-chart-visible-ticks"),
        ).toBe("13");
        const liveAlphaNetworkURLs = await alphaNetwork.readEventStreamURLs();
        const liveBetaNetworkURLs = await betaNetwork.readEventStreamURLs();
        expect(
          matchingTailURLs(liveAlphaNetworkURLs, alphaSessionID),
        ).toHaveLength(1);
        expect(
          matchingTailURLs(liveBetaNetworkURLs, betaSessionID),
        ).toHaveLength(1);
        expect(matchingTailURLs(liveAlphaNetworkURLs, betaSessionID)).toEqual(
          [],
        );
        expect(matchingTailURLs(liveBetaNetworkURLs, alphaSessionID)).toEqual(
          [],
        );

        await browserPage.page.evaluate(() => {
          window.dispatchEvent(new Event("pagehide"));
        });
        await waitForDurableCheckpoint(
          "alpha pagehide persistence of its exact live cursor",
          async () => {
            const envelope = await readCheckpointEnvelope(
              browserPage.page,
              alphaIdentity,
            );
            return (
              envelope?.checkpoint?.afterEventId === "alpha-live-18" &&
              envelope.checkpoint.afterSequence === 18 &&
              envelope.checkpoint.selectedTick === 21
            );
          },
        );
        await alphaNetwork.resetEventStreamURLs();
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });
        await selectSession(browserPage.page, "alpha");
        await waitForDurableCheckpoint(
          "alpha lifecycle resume from its exact live cursor",
          async () => {
            const urls = matchingTailURLs(
              await alphaNetwork.readEventStreamURLs(),
              alphaSessionID,
            );
            return (
              urls.length === 1 &&
              eventStreamURL(urls[0]).searchParams.get("after_event_id") ===
                "alpha-live-18" &&
              eventStreamURL(urls[0]).searchParams.get("after_sequence") ===
                "18"
            );
          },
        );
        await Promise.all([
          expectRestoredPage(browserPage.page, 21, "2,3,4,5,6,7,21"),
          expectRestoredPage(tabTwo, 13),
        ]);

        const [resumedAlpha, unchangedBeta] = await Promise.all([
          readCheckpointEnvelope(browserPage.page, alphaIdentity),
          readCheckpointEnvelope(tabTwo, betaIdentity),
        ]);
        expect(resumedAlpha.streamIdentity).toEqual(alphaIdentity);
        expect(resumedAlpha.checkpoint.afterEventId).toBe("alpha-live-18");
        expect(resumedAlpha.checkpoint.afterSequence).toBe(18);
        expect(resumedAlpha.checkpoint.selectedTick).toBe(21);
        expect(unchangedBeta.streamIdentity).toEqual(betaIdentity);
        expect(unchangedBeta.checkpoint.afterEventId).toBe(
          betaCheckpoint.afterEventId,
        );
        expect(unchangedBeta.checkpoint.afterSequence).toBe(
          betaCheckpoint.afterSequence,
        );
        expect(unchangedBeta.checkpoint.selectedTick).toBe(13);
        expect(
          await tabTwo
            .locator("[data-work-chart-visible-ticks]")
            .getAttribute("data-work-chart-visible-ticks"),
        ).toBe("13");
        expect(browserPage.pageErrors).toEqual([]);
        expect(browserPage.consoleErrors).toEqual([]);
        expect(tabTwoErrors.pageErrors).toEqual([]);
        expect(tabTwoErrors.consoleErrors).toEqual([]);
      } finally {
        if (browserPage?.page && !browserPage.page.isClosed()) {
          await clearTimelineCheckpoints(browserPage.page).catch(() => {});
        }
        await Promise.allSettled([
          tabTwo && !tabTwo.isClosed() ? tabTwo.close() : Promise.resolve(),
          browserPage?.close() ?? Promise.resolve(),
        ]);
        await server?.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
