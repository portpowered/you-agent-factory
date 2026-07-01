// @vitest-environment node

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  browserScenarioTimeoutMs,
  buildTimeoutMs,
  defaultFactorySessionID,
  expectNoBrowserErrors,
  initialEditableFactoryDefinitionVersion,
  openBrowserPage,
  startBrowserPreview,
  startFactoryApiServer,
  uiInteractionTimeoutMs,
  waitForDurableCheckpoint,
} from "./browser-test-harness.mjs";

const defaultFactoryDefinition = {
  name: "Manual Recovery Evidence Factory",
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
const resolvedFactorySessionID = "019e0000-0000-7000-8000-000000000042";

function buildStreamIdentity({
  backendScopeID = `${replayFactoryFolderPath}::browser-integration`,
  factorySessionID = defaultFactorySessionID,
  streamGenerationID = initialEditableFactoryDefinitionVersion.physical,
} = {}) {
  return {
    backendScopeID,
    factorySessionID,
    streamGenerationID,
  };
}

function checkpointStorageKey(identity) {
  return [
    identity.backendScopeID,
    identity.factorySessionID,
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

async function installNetworkCapture(page) {
  const captured = {
    eventStreamURLs: [],
    factorySessionReads: [],
  };

  await page.addInitScript(() => {
    window.__capturedEventStreamURLs = [];
    const OriginalEventSource = window.EventSource;
    window.EventSource = function EventSourceCapture(url, configuration) {
      window.__capturedEventStreamURLs.push(String(url));
      return new OriginalEventSource(url, configuration);
    };
    window.EventSource.prototype = OriginalEventSource.prototype;
  });

  await page.route("**/factory-sessions/**", async (route) => {
    const request = route.request();
    const url = request.url();
    if (
      request.method() === "GET" &&
      /\/factory-sessions\/[^/]+$/.test(new URL(url).pathname)
    ) {
      captured.factorySessionReads.push(url);
    }
    await route.continue();
  });

  return {
    captured,
    readEventStreamURLs: async () =>
      page.evaluate(() => window.__capturedEventStreamURLs ?? []),
    resetEventStreamURLs: async () => {
      await page.evaluate(() => {
        window.__capturedEventStreamURLs = [];
      });
    },
  };
}

async function clearTimelineCheckpoints(page) {
  await page.evaluate(async () => {
    const openRequest = indexedDB.open("agentFactoryTimelineCheckpoints", 2);
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
  });
}

async function seedTimelineCheckpoint(page, identity, cursor) {
  const storageKey = checkpointStorageKey(identity);
  await page.evaluate(
    async ({ envelope }) => {
      const openRequest = indexedDB.open(
        "agentFactoryTimelineCheckpoints",
        2,
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
      envelope: {
        checkpoint: {
          afterEventId: cursor.afterEventId,
          afterSequence: cursor.afterSequence,
          replayState: emptyReplayWorldState(cursor.selectedTick),
          selectedTick: cursor.selectedTick,
        },
        schemaVersion: 2,
        sessionID: identity.factorySessionID,
        storageKey,
        streamIdentity: identity,
      },
    },
  );
}

function eventStreamHasCursor(url, afterEventId) {
  return (
    url.includes(`after_event_id=${afterEventId}`) ||
    url.includes(`after_sequence=`)
  );
}

function eventStreamOmitsCursor(url) {
  return (
    !url.includes("after_event_id=") && !url.includes("after_sequence=")
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: manual recovery scenarios share one preview harness and IndexedDB helpers.
describe.sequential("dashboard session recovery manual scenarios", () => {
  let preview = null;

  beforeAll(async () => {
    preview = await startBrowserPreview();
  }, buildTimeoutMs);

  afterAll(async () => {
    await preview?.stop();
    preview = null;
  });

  it(
    "preserves a valid reconnect cursor across a backend restart when identity still matches",
    async () => {
      const identity = buildStreamIdentity();
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-restart-preserved-history",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await seedTimelineCheckpoint(browserPage.page, identity, {
          afterEventId: "manual-restart-event-5",
          afterSequence: 5,
          selectedTick: 5,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "restart cursor reuse",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "manual-restart-event-5"),
            );
          },
        );

        await network.resetEventStreamURLs();
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "second restart cursor reuse",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "manual-restart-event-5"),
            );
          },
        );

        const sessionReads = network.captured.factorySessionReads;
        expect(sessionReads.length).toBeGreaterThan(0);
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
    "drops stream-derived state after a clean restart when stream generation changes",
    async () => {
      const currentIdentity = buildStreamIdentity();
      const staleIdentity = buildStreamIdentity({
        streamGenerationID: "2026-01-01T00:00:00Z",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-clean-restart",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleIdentity, {
          afterEventId: "stale-clean-restart-event-11",
          afterSequence: 11,
          selectedTick: 11,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "clean restart without stale cursor",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) =>
            eventStreamHasCursor(url, "stale-clean-restart-event-11"),
          ),
        ).toBe(false);
        expect(currentIdentity.streamGenerationID).not.toBe(
          staleIdentity.streamGenerationID,
        );
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
    "remaps ~default to a resolved factorySessionID without reusing an old cursor",
    async () => {
      const remappedIdentity = buildStreamIdentity({
        factorySessionID: defaultFactorySessionID,
        streamGenerationID: initialEditableFactoryDefinitionVersion.physical,
      });
      const staleRemapIdentity = buildStreamIdentity({
        factorySessionID: resolvedFactorySessionID,
        streamGenerationID: "2026-06-29T12:00:00Z",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-session-remap",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleRemapIdentity, {
          afterEventId: "remap-stale-event-3",
          afterSequence: 3,
          selectedTick: 3,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "remap reconnect without old cursor",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) => eventStreamHasCursor(url, "remap-stale-event-3")),
        ).toBe(false);
        expect(remappedIdentity.factorySessionID).not.toBe(
          staleRemapIdentity.factorySessionID,
        );
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
    "never sends a stale cursor after switching backend scope",
    async () => {
      const localScopeIdentity = buildStreamIdentity({
        backendScopeID: "/local/factory::browser-integration",
      });
      const cloudScopeIdentity = buildStreamIdentity({
        backendScopeID: "/cloud/factory::browser-integration",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: {
          ...defaultFactoryDefinition,
          sourceDirectory: "/cloud/factory",
        },
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-backend-scope-switch",
      });

      try {
        const network = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, localScopeIdentity, {
          afterEventId: "local-scope-event-8",
          afterSequence: 8,
          selectedTick: 8,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "backend scope switch reconnect",
          async () => {
            const urls = await network.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const urls = await network.readEventStreamURLs();
        expect(
          urls.some((url) => eventStreamHasCursor(url, "local-scope-event-8")),
        ).toBe(false);
        expect(cloudScopeIdentity.backendScopeID).not.toBe(
          localScopeIdentity.backendScopeID,
        );
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
    "keeps tab-local reloads isolated while invalidating shared checkpoints on stream identity change",
    async () => {
      const identity = buildStreamIdentity();
      const staleIdentity = buildStreamIdentity({
        streamGenerationID: "2026-01-15T00:00:00Z",
      });
      const server = await startFactoryApiServer({
        apiPort: preview.apiPort,
        currentFactory: defaultFactoryDefinition,
        eventLines: [],
      });
      const browserPage = await openBrowserPage({
        artifactLabel: "manual-multi-tab-isolation",
      });

      try {
        const tabOneNetwork = await installNetworkCapture(browserPage.page);
        await browserPage.page.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });
        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, identity, {
          afterEventId: "multi-tab-event-4",
          afterSequence: 4,
          selectedTick: 4,
        });
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "tab one cursor reuse",
          async () => {
            const urls = await tabOneNetwork.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "multi-tab-event-4"),
            );
          },
        );

        const tabTwoPage = await browserPage.context.newPage();
        const tabTwoNetwork = await installNetworkCapture(tabTwoPage);
        await tabTwoPage.goto(preview.previewURL, {
          waitUntil: "domcontentloaded",
        });

        await waitForDurableCheckpoint(
          "tab two shared checkpoint cursor reuse",
          async () => {
            const urls = await tabTwoNetwork.readEventStreamURLs();
            return urls.some((url) =>
              eventStreamHasCursor(url, "multi-tab-event-4"),
            );
          },
          uiInteractionTimeoutMs,
        );

        await clearTimelineCheckpoints(browserPage.page);
        await seedTimelineCheckpoint(browserPage.page, staleIdentity, {
          afterEventId: "multi-tab-stale-event-9",
          afterSequence: 9,
          selectedTick: 9,
        });

        await tabOneNetwork.resetEventStreamURLs();
        await tabTwoNetwork.resetEventStreamURLs();
        await browserPage.page.reload({ waitUntil: "domcontentloaded" });
        await tabTwoPage.reload({ waitUntil: "domcontentloaded" });

        await waitForDurableCheckpoint(
          "tab one stale identity reconnect",
          async () => {
            const urls = await tabOneNetwork.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );
        await waitForDurableCheckpoint(
          "tab two stale identity reconnect",
          async () => {
            const urls = await tabTwoNetwork.readEventStreamURLs();
            return urls.some(
              (url) =>
                url.includes(
                  `/factory-sessions/${defaultFactorySessionID}/events`,
                ) && eventStreamOmitsCursor(url),
            );
          },
        );

        const tabOneURLs = await tabOneNetwork.readEventStreamURLs();
        const tabTwoURLs = await tabTwoNetwork.readEventStreamURLs();
        expect(
          tabOneURLs.some((url) =>
            eventStreamHasCursor(url, "multi-tab-stale-event-9"),
          ),
        ).toBe(false);
        expect(
          tabTwoURLs.some((url) =>
            eventStreamHasCursor(url, "multi-tab-stale-event-9"),
          ),
        ).toBe(false);

        await tabTwoPage.close();
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
