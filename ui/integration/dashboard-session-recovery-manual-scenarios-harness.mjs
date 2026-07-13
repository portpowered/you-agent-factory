import {
  emptyMaterializedWorkOutcomeState,
  initialEditableFactoryDefinitionVersion,
  resolvedDefaultFactorySessionID,
  timelineCheckpointDBVersion,
  timelineCheckpointSchemaVersion,
} from "./browser-test-harness.mjs";

export const defaultFactoryDefinition = {
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

export const replayFactoryFolderPath = "/replay/factory";
export const resolvedFactorySessionID = resolvedDefaultFactorySessionID;
const defaultLogicalSessionKeyID = `${replayFactoryFolderPath}::default::`;

export function buildStreamIdentity({
  backendScopeID = `${replayFactoryFolderPath}::browser-integration`,
  factorySessionID = resolvedDefaultFactorySessionID,
  logicalSessionKeyID = defaultLogicalSessionKeyID,
  streamGenerationID = initialEditableFactoryDefinitionVersion.physical,
} = {}) {
  return {
    backendScopeID,
    factorySessionID,
    logicalSessionKeyID,
    streamGenerationID,
  };
}

export function checkpointStorageKey(identity) {
  return [
    identity.backendScopeID,
    identity.factorySessionID,
    identity.logicalSessionKeyID,
    identity.streamGenerationID,
  ].join("::");
}

export function emptyReplayWorldState(tick) {
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

export function replayWorldStateWithProviderSessionRef(tick, providerSessionRef) {
  return {
    ...emptyReplayWorldState(tick),
    providerSessions: [
      {
        providerSessionRef,
        status: "ACTIVE",
      },
    ],
  };
}

export async function installNetworkCapture(page) {
  const captured = {
    eventStreamURLs: [],
    factorySessionReads: [],
    syncPreflightReads: [],
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
    if (
      request.method() === "GET" &&
      /\/factory-sessions\/[^/]+\/sync-preflight/.test(new URL(url).pathname)
    ) {
      captured.syncPreflightReads.push(url);
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

export async function clearTimelineCheckpoints(page) {
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

export async function seedTimelineCheckpoint(page, identity, cursor) {
  const storageKey = checkpointStorageKey(identity);
  const replayState =
    cursor.replayState ??
    emptyReplayWorldState(cursor.selectedTick ?? cursor.afterSequence ?? 0);
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
          replayState,
          selectedTick: cursor.selectedTick,
        },
        schemaVersion: timelineCheckpointSchemaVersion,
        storageKey,
        streamIdentity: identity,
      },
    },
  );
}

export function eventStreamHasCursor(url, afterEventId) {
  return (
    url.includes(`after_event_id=${afterEventId}`) ||
    url.includes(`after_sequence=`)
  );
}

export function eventStreamOmitsCursor(url) {
  return (
    !url.includes("after_event_id=") && !url.includes("after_sequence=")
  );
}
