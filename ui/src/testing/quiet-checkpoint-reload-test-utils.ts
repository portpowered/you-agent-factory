import { waitFor } from "@testing-library/react";
import { vi } from "vitest";

import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";
import { emptyReplayWorldState } from "../features/timeline/state/timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../features/timeline/state/timeline/storeState";
import {
  persistTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../features/timeline/state/timelineCheckpointPersistence";
import type { RenderAppFetchOverride } from "./app-shell-fetch-test-utils";
import { APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID } from "./app-shell-session-preflight-test-utils";
import { MockEventSource } from "./app-shell-session-stream-test-utils";

const BACKEND_SCOPE_ID = "/workspace::test-backend";
const LOGICAL_SESSION_KEY_ID = "lsk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const STREAM_GENERATION_ID = "2026-06-26T00:00:00Z";

interface StoredCheckpointEnvelope {
  storageKey?: string;
}

export interface QuietCheckpointReloadObservations {
  checkpointHydrated: boolean;
  eventArrived: boolean;
  preflightCompleted: boolean;
  streamOpened: boolean;
}

export interface QuietCheckpointReloadFixture {
  completePreflight: () => void;
  fetchOverride: RenderAppFetchOverride;
  hydrateCheckpoint: () => void;
  installCheckpoint: () => Promise<void>;
  observations: () => QuietCheckpointReloadObservations;
  openStream: (stream?: MockEventSource) => MockEventSource;
  scenario: {
    checkpoint: FactoryTimelineCheckpoint;
    streamIdentity: TimelineCheckpointStreamIdentity;
    tick: number;
  };
  waitForCheckpointHydration: () => Promise<void>;
  waitForStreamCreation: () => Promise<MockEventSource>;
}

function indexedDBRequest<T>(result: T, beforeSuccess?: () => void) {
  const request = {
    error: null,
    onblocked: null,
    onerror: null,
    onsuccess: null,
    onupgradeneeded: null,
    result,
  } as unknown as IDBRequest<T> & {
    onupgradeneeded?: ((event: IDBVersionChangeEvent) => void) | null;
  };

  queueMicrotask(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
  });
  return request;
}

function createIndexedDBTestDouble(): IDBFactory {
  const records = new Map<string, StoredCheckpointEnvelope>();
  const database = {
    close: () => {},
    createObjectStore: () => undefined,
    deleteObjectStore: () => undefined,
    objectStoreNames: { contains: () => true },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => records.delete(key)),
        get: (key: string) => indexedDBRequest(records.get(key)),
        getAll: () => indexedDBRequest([...records.values()]),
        put: (value: StoredCheckpointEnvelope) =>
          indexedDBRequest(value.storageKey ?? "", () => {
            if (value.storageKey) {
              records.set(value.storageKey, value);
            }
          }),
      }),
    }),
  };

  return {
    open: () => {
      const request = indexedDBRequest(database);
      queueMicrotask(() =>
        request.onupgradeneeded?.({} as IDBVersionChangeEvent),
      );
      return request;
    },
  } as unknown as IDBFactory;
}

function preflightResponse(checkpoint: FactoryTimelineCheckpoint): Response {
  return new Response(
    JSON.stringify({
      backendScopeId: BACKEND_SCOPE_ID,
      checkpointReusable: true,
      factorySessionId: APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID,
      logicalSessionKeyId: LOGICAL_SESSION_KEY_ID,
      reasonCode: "ok",
      reconnectCursor: {
        afterEventId: checkpoint.afterEventId,
        afterSequence: checkpoint.afterSequence,
        provided: true,
        validForStreamGeneration: true,
      },
      requestedSessionId: "~default",
      streamGenerationId: STREAM_GENERATION_ID,
    }),
    { headers: { "Content-Type": "application/json" } },
  );
}

export function createQuietCheckpointReloadFixture(
  tick: number,
): QuietCheckpointReloadFixture {
  const replayState = emptyReplayWorldState(tick);
  replayState.factory_state = `CHECKPOINT_CURRENT_AT_${tick}`;
  replayState.runtime.session.has_data = true;

  const checkpoint: FactoryTimelineCheckpoint = {
    afterEventId: `quiet-checkpoint-event-${tick}`,
    afterSequence: tick,
    replayState,
    selectedTick: tick,
    syncIdentity: {
      backendScopeId: BACKEND_SCOPE_ID,
      factorySessionId: APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID,
      logicalSessionKeyId: LOGICAL_SESSION_KEY_ID,
      streamGenerationId: STREAM_GENERATION_ID,
    },
  };
  const streamIdentity: TimelineCheckpointStreamIdentity = {
    backendScopeID: BACKEND_SCOPE_ID,
    factorySessionID: APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyID: LOGICAL_SESSION_KEY_ID,
    streamGenerationID: STREAM_GENERATION_ID,
  };
  const observed = {
    checkpointHydrated: false,
    preflightCompleted: false,
  };
  let releasePreflight: (() => void) | undefined;
  const preflightGate = new Promise<void>((resolve) => {
    releasePreflight = resolve;
  });

  const fetchOverride: RenderAppFetchOverride = async (
    path,
    method,
    _input,
    init,
  ) => {
    if (method === "GET" && path.includes("/sync-preflight")) {
      await preflightGate;
      observed.preflightCompleted = true;
      return preflightResponse(checkpoint);
    }
    if (
      method === "GET" &&
      path.includes("/events?") &&
      new Headers(init?.headers).get("Accept") === "text/event-stream"
    ) {
      return new Response(null, { status: 200 });
    }
    return undefined;
  };

  return {
    completePreflight: () => releasePreflight?.(),
    fetchOverride,
    hydrateCheckpoint: () => {
      useFactoryTimelineStore.getState().restoreCheckpoint(checkpoint);
      observed.checkpointHydrated = true;
    },
    installCheckpoint: async () => {
      const indexedDB = createIndexedDBTestDouble();
      vi.stubGlobal("indexedDB", indexedDB);
      await persistTimelineCheckpoint(indexedDB, checkpoint, streamIdentity);
    },
    observations: () => {
      const stream = MockEventSource.instances.at(-1);
      return {
        checkpointHydrated: observed.checkpointHydrated,
        eventArrived: (stream?.messageEventCount ?? 0) > 0,
        preflightCompleted: observed.preflightCompleted,
        streamOpened: stream?.opened ?? false,
      };
    },
    openStream: (stream = MockEventSource.instances.at(-1)) => {
      if (!stream) {
        throw new Error("expected quiet reload event stream to be created");
      }
      stream.open();
      return stream;
    },
    scenario: { checkpoint, streamIdentity, tick },
    waitForCheckpointHydration: async () => {
      await waitFor(() => {
        const state = useFactoryTimelineStore.getState();
        if (
          state.currentReplayCheckpoint?.afterEventId !==
            checkpoint.afterEventId ||
          state.selectedTick !== tick ||
          state.worldViewCache[tick]?.factory_state !==
            replayState.factory_state
        ) {
          throw new Error("quiet reload checkpoint has not hydrated");
        }
      });
      observed.checkpointHydrated = true;
    },
    waitForStreamCreation: async () => {
      let stream: MockEventSource | undefined;
      await waitFor(() => {
        stream = MockEventSource.instances.at(-1);
        if (!stream) {
          throw new Error("quiet reload event stream has not been created");
        }
      });
      return stream as MockEventSource;
    },
  };
}
