import { initialEditableFactoryDefinitionVersion } from "./browser-test-harness.mjs";
import {
  checkpointStorageKey,
  emptyReplayWorldState,
} from "./dashboard-session-recovery-manual-scenarios-harness.mjs";

export const alphaSessionID = "11111111-1111-4111-8111-111111111111";
export const betaSessionID = "22222222-2222-4222-8222-222222222222";

export function sessionFixture(id, name) {
  return {
    factoryDir: `/workspace/${name}`,
    folderPath: `/workspace/${name}`,
    id,
    isDefault: false,
    project: name,
    target: { kind: "named", name },
  };
}

export function streamIdentity(sessionID, name, streamGenerationID) {
  return {
    backendScopeID: `/workspace/${name}::browser-integration`,
    factorySessionID: sessionID,
    logicalSessionKeyID: `/workspace/${name}::named::${name}`,
    streamGenerationID,
  };
}

export function checkpointFixture({
  eventID,
  sequence,
  tick,
  sampleTicks = [tick],
  value,
}) {
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
      samples: sampleTicks.map((sampleTick, index) => {
        const sampleValue = Math.max(0, value - sampleTicks.length + index + 1);
        return {
          completedCount: sampleValue,
          dispatchedCount: sampleValue,
          failedByWorkType: {},
          failedCount: 0,
          failedWorkLabels: [],
          inFlightCount: 0,
          observedAt: sampleTick * 1_000,
          queuedCount: 0,
          tick: sampleTick,
        };
      }),
      version: 1,
    },
    replayState,
    selectedTick: tick,
  };
}

export function tailEvent({ eventID, sequence, tick }) {
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

export function eventStreamURL(url) {
  return new URL(url, "http://browser-integration.invalid");
}

export function matchingTailURLs(urls, sessionID) {
  return urls.filter((url) => {
    const parsed = eventStreamURL(url);
    return parsed.pathname.endsWith(`/factory-sessions/${sessionID}/events`);
  });
}

export async function readCheckpointEnvelope(page, identity) {
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

export async function deleteCheckpointEnvelope(page, identity) {
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
