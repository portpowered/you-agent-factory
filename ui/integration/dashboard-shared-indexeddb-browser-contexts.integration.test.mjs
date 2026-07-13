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
  checkpointStorageKey,
  clearTimelineCheckpoints,
  defaultFactoryDefinition,
  emptyReplayWorldState,
  installNetworkCapture,
  seedTimelineCheckpoint,
} from "./dashboard-session-recovery-manual-scenarios-harness.mjs";

const alphaSessionID = "11111111-1111-4111-8111-111111111111";
const betaSessionID = "22222222-2222-4222-8222-222222222222";

function sessionFixture(id, name) {
  return {
    factoryDir: `/workspace/${name}`,
    folderPath: `/workspace/${name}`,
    id,
    isDefault: false,
    project: name,
    target: { kind: "named", name },
  };
}

function streamIdentity(sessionID, name, streamGenerationID) {
  return {
    backendScopeID: `/workspace/${name}::browser-integration`,
    factorySessionID: sessionID,
    logicalSessionKeyID: `/workspace/${name}::named::${name}`,
    streamGenerationID,
  };
}

function checkpointFixture({ eventID, sequence, tick, value }) {
  const replayState = emptyReplayWorldState(tick);
  replayState.runtime.dispatched_count = value;
  replayState.runtime.has_data = true;
  return {
    afterEventId: eventID,
    afterSequence: sequence,
    materializedWorkOutcomeState: {
      accumulator: {
        activeDispatchesByID: {},
        appliedEventCount: value,
        completedAcceptedCount: value,
        completedDispatchCount: value,
        failedWorkItemsByID: {},
        initialPlaceIDs: ["story:queued"],
        workItemsByID: {},
      },
      counts: {
        completed: value,
        dispatched: value,
        failed: 0,
        inFlight: 0,
        queued: 0,
      },
      cursor: {
        eventID,
        eventTime: initialEditableFactoryDefinitionVersion.physical,
        sequence,
        tick,
      },
      failedByWorkType: {},
      failedWorkLabels: [],
      samples: [
        {
          completedCount: value,
          dispatchedCount: value,
          failedByWorkType: {},
          failedCount: 0,
          failedWorkLabels: [],
          inFlightCount: 0,
          observedAt: tick * 1_000,
          queuedCount: 0,
          tick,
        },
      ],
      version: 1,
    },
    replayState,
    selectedTick: tick,
  };
}

function tailEvent({ eventID, sequence, tick }) {
  return JSON.stringify({
    context: {
      eventTime: `2026-07-13T17:00:${String(tick).padStart(2, "0")}Z`,
      sequence,
      tick,
    },
    id: eventID,
    payload: {
      previousState: "RUNNING",
      reason: "isolated browser tail",
      state: "FINISHED",
    },
    type: "FACTORY_STATE_RESPONSE",
  });
}

function matchingTailURLs(urls, sessionID) {
  return urls.filter((url) => {
    const parsed = eventStreamURL(url);
    return parsed.pathname.endsWith(`/factory-sessions/${sessionID}/events`);
  });
}

function eventStreamURL(url) {
  return new URL(url, "http://browser-integration.invalid");
}

async function selectSession(page, name) {
  const tab = page.getByRole("tab", { name });
  await tab.waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  await tab.click();
}

async function readCheckpointEnvelope(page, identity) {
  return page.evaluate(
    async ({ key }) => {
      const request = indexedDB.open("agentFactoryTimelineCheckpoints", 3);
      const database = await new Promise((resolve, reject) => {
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      });
      try {
        return await new Promise((resolve, reject) => {
          const read = database
            .transaction("checkpoints", "readonly")
            .objectStore("checkpoints")
            .get(key);
          read.onsuccess = () => resolve(read.result ?? null);
          read.onerror = () => reject(read.error);
        });
      } finally {
        database.close();
      }
    },
    { key: checkpointStorageKey(identity) },
  );
}

async function deleteCheckpointEnvelope(page, identity) {
  await page.evaluate(
    async ({ key }) => {
      const request = indexedDB.open("agentFactoryTimelineCheckpoints", 3);
      const database = await new Promise((resolve, reject) => {
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
      });
      try {
        await new Promise((resolve, reject) => {
          const transaction = database.transaction("checkpoints", "readwrite");
          const deletion = transaction.objectStore("checkpoints").delete(key);
          deletion.onerror = () => reject(deletion.error);
          transaction.oncomplete = () => resolve();
          transaction.onerror = () => reject(transaction.error);
          transaction.onabort = () => reject(transaction.error);
        });
      } finally {
        database.close();
      }
    },
    { key: checkpointStorageKey(identity) },
  );
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
    "restores and tails two concrete sessions without cross-tab checkpoint contamination",
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
        sequence: 17,
        tick: 7,
        value: 3,
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
            ],
            [betaSessionID]: [
              tailEvent({
                eventID: betaCheckpoint.afterEventId,
                sequence: betaCheckpoint.afterSequence,
                tick: betaCheckpoint.selectedTick,
              }),
              tailEvent({ eventID: "beta-live-30", sequence: 30, tick: 22 }),
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
          expectRestoredPage(browserPage.page, 7),
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
          expectRestoredPage(browserPage.page, 21, "7,21"),
          expectRestoredPage(tabTwo, 22, "13,22"),
        ]);
        expect(
          await browserPage.page
            .locator("[data-work-chart-visible-ticks]")
            .getAttribute("data-work-chart-visible-ticks"),
        ).toBe("7,21");
        expect(
          await tabTwo
            .locator("[data-work-chart-visible-ticks]")
            .getAttribute("data-work-chart-visible-ticks"),
        ).toBe("13,22");
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
