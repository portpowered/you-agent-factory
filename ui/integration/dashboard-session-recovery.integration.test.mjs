// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  emptyMaterializedWorkOutcomeState,
  expectNoBrowserErrors,
  initialEditableFactoryDefinitionVersion,
  openBrowserPage,
  openDashboardWithSeededCheckpoint,
  resolvedDefaultFactorySessionID,
  startBrowserPreview,
  startFactoryApiServer,
  timelineCheckpointDBVersion,
  timelineCheckpointSchemaVersion,
  uiInteractionTimeoutMs,
  waitForDurableCheckpoint,
} from "./browser-test-harness.mjs";

const defaultFactoryDefinition = {
  name: "Browser Recovery Harness Factory",
  workers: [],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [],
};

const replayFactoryFolderPath = "/replay/factory";
const defaultLogicalSessionKeyID = `${replayFactoryFolderPath}::default::`;
const currentStreamIdentity = {
  backendScopeID: `${replayFactoryFolderPath}::browser-integration`,
  factorySessionID: resolvedDefaultFactorySessionID,
  logicalSessionKeyID: defaultLogicalSessionKeyID,
  streamGenerationID: initialEditableFactoryDefinitionVersion.physical,
};

function checkpointStorageKey(identity) {
  return [
    identity.backendScopeID,
    identity.factorySessionID,
    identity.logicalSessionKeyID,
    identity.streamGenerationID,
  ].join("::");
}

function emptyReplayWorldState(tick) {
  return {
    activeDispatches: {},
    completedDispatches: [],
    factory_state: "UNKNOWN",
    failedWorkDetailsByWorkID: {},
    failedWorkItemsByID: {},
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {},
    payloadLineage: {
      edgesByWorkID: {},
      nodesByID: {},
    },
    providerSessions: [],
    relationsByWorkID: {},
    runtime: {
      categories: {},
      factoryState: "UNKNOWN",
      inFlightCount: 0,
      totalTokens: 0,
    },
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    sessionArtifacts: [],
    terminalWorkByID: {},
    textBlobsByID: {},
    tick_count: tick,
    topology: {},
    tracesByID: {},
    tracesByWorkID: {},
    uptime_seconds: 0,
    workItemsByID: {},
    workStateChangesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

async function installEventStreamCapture(page) {
  await page.addInitScript(() => {
    window.__capturedEventStreamURLs = [];
    const OriginalEventSource = window.EventSource;
    window.EventSource = function EventSourceCapture(url, configuration) {
      window.__capturedEventStreamURLs.push(String(url));
      return new OriginalEventSource(url, configuration);
    };
    window.EventSource.prototype = OriginalEventSource.prototype;
  });
}

async function clearTimelineCheckpoints(page) {
  await page.evaluate(async (dbVersion) => {
    const openRequest = indexedDB.open(
      "agentFactoryTimelineCheckpoints",
      dbVersion,
    );
    const database = await new Promise((resolve, reject) => {
      openRequest.onupgradeneeded = () => {
        const db = openRequest.result;
        if (!db.objectStoreNames.contains("checkpoints")) {
          db.createObjectStore("checkpoints", { keyPath: "storageKey" });
        }
      };
      openRequest.onsuccess = () => resolve(openRequest.result);
      openRequest.onerror = () => reject(openRequest.error);
    });
    await new Promise((resolve, reject) => {
      const transaction = database.transaction("checkpoints", "readwrite");
      const request = transaction.objectStore("checkpoints").clear();
      request.onsuccess = () => resolve(undefined);
      request.onerror = () => reject(request.error);
    });
    database.close();
  }, timelineCheckpointDBVersion);
}

async function seedTimelineCheckpoint(page, identity, cursor) {
  const storageKey = checkpointStorageKey(identity);
  await page.evaluate(
    async ({ dbVersion, envelope }) => {
      const openRequest = indexedDB.open(
        "agentFactoryTimelineCheckpoints",
        dbVersion,
      );
      const database = await new Promise((resolve, reject) => {
        openRequest.onupgradeneeded = () => {
          const db = openRequest.result;
          if (!db.objectStoreNames.contains("checkpoints")) {
            db.createObjectStore("checkpoints", { keyPath: "storageKey" });
          }
        };
        openRequest.onsuccess = () => resolve(openRequest.result);
        openRequest.onerror = () => reject(openRequest.error);
      });
      await new Promise((resolve, reject) => {
        const transaction = database.transaction("checkpoints", "readwrite");
        const store = transaction.objectStore("checkpoints");
        const request = store.put(envelope);
        request.onsuccess = () => resolve(undefined);
        request.onerror = () => reject(request.error);
      });
      database.close();
    },
    {
      dbVersion: timelineCheckpointDBVersion,
      envelope: {
        checkpoint: {
          afterEventId: cursor.afterEventId,
          afterSequence: cursor.afterSequence,
          materializedWorkOutcomeState:
            emptyMaterializedWorkOutcomeState(cursor),
          replayState: emptyReplayWorldState(cursor.selectedTick),
          selectedTick: cursor.selectedTick,
        },
        schemaVersion: timelineCheckpointSchemaVersion,
        storageKey,
        streamIdentity: identity,
      },
    },
  );
}

async function readCapturedEventStreamURLs(page) {
  return page.evaluate(() => window.__capturedEventStreamURLs ?? []);
}

async function assertRecoveryPanelVisible(page, viewport) {
  await page.setViewportSize(viewport);
  await page
    .getByRole("heading", { name: "Session replay needs attention" })
    .waitFor({ state: "visible", timeout: uiInteractionTimeoutMs });
  const retryButton = page.getByRole("button", {
    name: "Retry session stream",
  });
  await retryButton.waitFor({
    state: "visible",
    timeout: uiInteractionTimeoutMs,
  });
  await retryButton.focus();
  await expect
    .poll(async () =>
      retryButton.evaluate((node) => node === document.activeElement),
    )
    .toBe(true);
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: browser recovery scenarios share one preview harness and IndexedDB seeding helpers.
describe.sequential("dashboard session recovery browser integration", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "reuses a matching checkpoint cursor and ignores stale stream identity checkpoints",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "dashboard-session-recovery-identity",
      });

      try {
        await installEventStreamCapture(browserPage.page);
        await openDashboardWithSeededCheckpoint(
          browserPage.page,
          preview.previewURL,
          () =>
            seedTimelineCheckpoint(browserPage.page, currentStreamIdentity, {
              afterEventId: "browser-checkpoint-event-7",
              afterSequence: 7,
              selectedTick: 7,
            }),
        );
        await waitForDurableCheckpoint(
          "matching checkpoint stream",
          async () => {
            const urls = await readCapturedEventStreamURLs(browserPage.page);
            return urls.some((url) =>
              url.includes("after_event_id=browser-checkpoint-event-7"),
            );
          },
        );

        const matchingURLs = await readCapturedEventStreamURLs(
          browserPage.page,
        );
        expect(
          matchingURLs.some((url) =>
            url.includes("after_event_id=browser-checkpoint-event-7"),
          ),
        ).toBe(true);

        await browserPage.page.reload({ waitUntil: "domcontentloaded" });
        await new Promise((resolve) => setTimeout(resolve, 800));
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(
          browserPage.page,
          {
            ...currentStreamIdentity,
            streamGenerationID: "stale-stream-generation",
          },
          {
            afterEventId: "stale-checkpoint-event-9",
            afterSequence: 9,
            selectedTick: 9,
          },
        );
        await browserPage.page.evaluate(() => {
          window.__capturedEventStreamURLs = [];
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "stale identity reconnect",
          async () => {
            const urls = await readCapturedEventStreamURLs(browserPage.page);
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
                ) &&
                !url.includes("after_event_id=") &&
                !url.includes("after_sequence="),
            );
          },
          uiInteractionTimeoutMs * 2,
        );

        const staleURLs = await readCapturedEventStreamURLs(browserPage.page);
        expect(
          staleURLs.some(
            (url) =>
              url.includes(
                `/factory-sessions/${resolvedDefaultFactorySessionID}/events`,
              ) && !url.includes("after_event_id=stale-checkpoint-event-9"),
          ),
        ).toBe(true);
        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );

  it(
    "shows a distinct recoverable replay failure state at desktop and narrow viewports",
    async () => {
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "dashboard-session-recovery-ui",
      });

      try {
        await installEventStreamCapture(browserPage.page);
        await openDashboardWithSeededCheckpoint(
          browserPage.page,
          preview.previewURL,
          () =>
            seedTimelineCheckpoint(browserPage.page, currentStreamIdentity, {
              afterEventId: "browser-checkpoint-event-7",
              afterSequence: 7,
              selectedTick: 7,
            }),
        );

        let eventStreamAttempts = 0;
        await browserPage.page.route(
          `**/factory-sessions/${resolvedDefaultFactorySessionID}/events**`,
          async (route) => {
            const request = route.request();
            const acceptHeader = request.headers().accept ?? "";
            const url = request.url();
            const hasCursor =
              url.includes("after_event_id=") ||
              url.includes("after_sequence=");

            if (acceptHeader.includes("application/json")) {
              await route.fulfill({
                body: JSON.stringify({
                  factorySessionId: resolvedDefaultFactorySessionID,
                  outcome: "CURSOR_STALE",
                  retry: {
                    omitAfterEventId: true,
                    omitAfterSequence: true,
                  },
                }),
                contentType: "application/json",
                status: 200,
              });
              return;
            }

            if (request.resourceType() === "fetch" && hasCursor) {
              await route.fulfill({
                body: "event: validation\n\n",
                contentType: "text/event-stream",
                status: 200,
              });
              return;
            }

            eventStreamAttempts += 1;
            await route.abort("failed");
          },
        );

        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "recovery failure panel",
          async () => {
            try {
              await browserPage.page
                .getByRole("heading", {
                  name: "Session replay needs attention",
                })
                .waitFor({ state: "visible", timeout: 1_000 });
              return true;
            } catch {
              return false;
            }
          },
          uiInteractionTimeoutMs,
        );

        expect(eventStreamAttempts).toBeGreaterThanOrEqual(2);
        expect(
          await browserPage.page
            .getByRole("heading", { name: "Dashboard unavailable" })
            .count(),
        ).toBe(0);

        await assertRecoveryPanelVisible(browserPage.page, {
          height: 900,
          width: 1280,
        });
        await assertRecoveryPanelVisible(browserPage.page, {
          height: 700,
          width: 390,
        });

        expectNoBrowserErrors(
          browserPage.pageErrors,
          browserPage.consoleErrors,
          expect,
        );
      } finally {
        await browserPage.close();
        await server.stop();
      }
    },
    browserScenarioTimeoutMs,
  );
});
