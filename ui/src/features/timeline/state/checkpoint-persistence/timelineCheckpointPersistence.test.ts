import { describe, expect, it, vi } from "vitest";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import {
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../../../dashboard/public/session-persistence-diagnostics";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  clearTimelineCheckpoint,
  persistTimelineCheckpoint,
  purgeLegacyTimelineCheckpoints,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint?: {
    afterEventId?: string;
    afterSequence?: number;
    replayState?: ReturnType<typeof emptyReplayWorldState>;
    selectedTick: number;
  };
  schemaVersion: number;
  sessionID?: string;
  storageKey?: string;
  streamIdentity?: TimelineCheckpointStreamIdentity;
}

const RESOLVED_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function createIndexedDBTestDouble(
  options: {
    failOpen?: boolean;
    failPut?: boolean;
    storeExists?: boolean;
  } = {},
) {
  const records = new Map<string, StoredCheckpointEnvelope>();
  const createObjectStore = vi.fn();
  const database = {
    close: () => {},
    createObjectStore,
    deleteObjectStore: vi.fn(),
    objectStoreNames: {
      contains: () => options.storeExists ?? true,
    },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => {
            records.delete(key);
          }),
        get: (key: string) => indexedDBRequest(records.get(key)),
        put: (value: StoredCheckpointEnvelope) =>
          options.failPut
            ? indexedDBErrorRequest<string>(new Error("put failed"))
            : indexedDBRequest(value.storageKey ?? "", () => {
                if (value.storageKey) {
                  records.set(value.storageKey, value);
                }
              }),
      }),
    }),
  };

  return {
    indexedDB: {
      open: () => {
        if (options.failOpen) {
          return indexedDBErrorRequest<typeof database>(
            new Error("open failed"),
          );
        }

        const request = indexedDBRequest(database);
        queueMicrotask(() =>
          request.onupgradeneeded?.({} as IDBVersionChangeEvent),
        );
        return request;
      },
    } as unknown as IDBFactory,
    createObjectStore,
    records,
  };
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
    onblocked?: ((event: Event) => void) | null;
    onupgradeneeded?: ((event: IDBVersionChangeEvent) => void) | null;
  };

  queueMicrotask(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
  });

  return request;
}

function indexedDBErrorRequest<T>(error: Error) {
  const request = {
    error,
    onblocked: null,
    onerror: null,
    onsuccess: null,
    onupgradeneeded: null,
    result: undefined,
  } as unknown as IDBRequest<T> & {
    onblocked?: ((event: Event) => void) | null;
    onupgradeneeded?: ((event: IDBVersionChangeEvent) => void) | null;
  };

  queueMicrotask(() => {
    request.onerror?.({} as Event);
  });

  return request;
}

function checkpointFixture(): FactoryTimelineCheckpoint {
  const replayState = emptyReplayWorldState(7);
  replayState.textBlobsByID = {
    long: "x".repeat(600),
    short: "kept",
  };

  return {
    afterEventId: "event-7",
    afterSequence: 42,
    replayState,
    selectedTick: 7,
    syncIdentity: {
      backendScopeId: "backend-a",
      factorySessionId: RESOLVED_SESSION_UUID,
      logicalSessionKeyId: "logical-a",
      streamGenerationId: "stream-a",
    },
  };
}

function streamIdentityFixture(): TimelineCheckpointStreamIdentity {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: RESOLVED_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

function checkpointStorageKey(
  identity: TimelineCheckpointStreamIdentity,
): string {
  return [
    identity.backendScopeID,
    identity.factorySessionID,
    identity.streamGenerationID,
  ].join("::");
}

describe("timeline checkpoint persistence", () => {
  it("persists compact checkpoints without mutating the live replay state", async () => {
    const { indexedDB, records } = createIndexedDBTestDouble();
    const checkpoint = checkpointFixture();
    const streamIdentity = streamIdentityFixture();

    await persistTimelineCheckpoint(indexedDB, checkpoint, streamIdentity);
    const restored = await readTimelineCheckpoint(indexedDB, streamIdentity);

    expect(records.has(checkpointStorageKey(streamIdentity))).toBe(true);
    expect(records.has(DEFAULT_FACTORY_SESSION_ID)).toBe(false);
    expect(checkpoint.replayState.textBlobsByID.long).toHaveLength(600);
    expect(restored?.selectedTick).toBe(7);
    expect(restored?.afterEventId).toBe("event-7");
    expect(restored?.afterSequence).toBe(42);
    expect(restored?.replayState.textBlobsByID.short).toBe("kept");
    expect(restored?.replayState.textBlobsByID.long).toContain(
      "[checkpoint truncated 88 chars]",
    );
    expect(restored?.replayState.textBlobsByID.long.length).toBeLessThan(600);
    expect(restored?.syncIdentity).toEqual({
      backendScopeId: "backend-a",
      factorySessionId: RESOLVED_SESSION_UUID,
      logicalSessionKeyId: "logical-a",
      streamGenerationId: "stream-a",
    });
  });
});

describe("timeline checkpoint persistence diagnostics", () => {
  it("records user-initiated checkpoint clears through session persistence diagnostics", async () => {
    resetSessionPersistenceInvalidationRecords();
    const { indexedDB, records } = createIndexedDBTestDouble();
    const streamIdentity = streamIdentityFixture();

    await persistTimelineCheckpoint(
      indexedDB,
      checkpointFixture(),
      streamIdentity,
    );
    await clearTimelineCheckpoint(indexedDB, streamIdentity, {
      requestedSessionID: streamIdentity.factorySessionID,
      userInitiated: true,
    });

    expect(records.has(checkpointStorageKey(streamIdentity))).toBe(false);
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      {
        reason: "user_cleared_sessions",
        recoveryAction: "clear_checkpoint",
        requestedSessionID: streamIdentity.factorySessionID,
        scope: {
          backendScopeID: streamIdentity.backendScopeID,
          factorySessionID: streamIdentity.factorySessionID,
          streamGenerationID: streamIdentity.streamGenerationID,
        },
      },
    ]);
  });

  it("records identity mismatch diagnostics when stored checkpoint scope drifts", async () => {
    resetSessionPersistenceInvalidationRecords();
    const { indexedDB, records } = createIndexedDBTestDouble();
    const streamIdentity = streamIdentityFixture();
    const storageKey = checkpointStorageKey(streamIdentity);

    records.set(storageKey, {
      checkpoint: checkpointFixture(),
      schemaVersion: 3,
      storageKey,
      streamIdentity: {
        ...streamIdentity,
        streamGenerationID: "2026-06-27T00:00:00Z",
      },
    });

    await expect(
      readTimelineCheckpoint(indexedDB, streamIdentity),
    ).resolves.toBe(null);
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      expect.objectContaining({
        reason: "stream_generation_changed",
        recoveryAction: "clear_stream_derived_state",
        requestedSessionID: streamIdentity.factorySessionID,
      }),
    ]);
  });
});

describe("timeline checkpoint persistence resilience", () => {
  it("drops invalid stored checkpoints and ignores missing persistence inputs", async () => {
    const { indexedDB, records } = createIndexedDBTestDouble();
    records.set(checkpointStorageKey(streamIdentityFixture()), {
      checkpoint: {
        replayState: emptyReplayWorldState(1),
        selectedTick: 1,
      },
      schemaVersion: 999,
      storageKey: checkpointStorageKey(streamIdentityFixture()),
      streamIdentity: streamIdentityFixture(),
    });

    await expect(
      readTimelineCheckpoint(undefined, streamIdentityFixture()),
    ).resolves.toBe(null);
    await expect(readTimelineCheckpoint(indexedDB, null)).resolves.toBe(null);
    await persistTimelineCheckpoint(indexedDB, undefined, null);

    await expect(
      readTimelineCheckpoint(indexedDB, streamIdentityFixture()),
    ).resolves.toBe(null);
    expect(records.has(checkpointStorageKey(streamIdentityFixture()))).toBe(
      false,
    );
  });

  it("creates the checkpoint store during database upgrades", async () => {
    const { createObjectStore, indexedDB } = createIndexedDBTestDouble({
      storeExists: false,
    });

    await persistTimelineCheckpoint(
      indexedDB,
      checkpointFixture(),
      streamIdentityFixture(),
    );

    expect(createObjectStore).toHaveBeenCalledWith("checkpoints", {
      keyPath: "storageKey",
    });
  });

  it("cleans stale checkpoint data when IndexedDB operations fail", async () => {
    const writeFailure = createIndexedDBTestDouble({ failPut: true });
    writeFailure.records.set(checkpointStorageKey(streamIdentityFixture()), {
      checkpoint: {
        replayState: emptyReplayWorldState(1),
        selectedTick: 1,
      },
      schemaVersion: 1,
      storageKey: checkpointStorageKey(streamIdentityFixture()),
      streamIdentity: streamIdentityFixture(),
    });

    await persistTimelineCheckpoint(
      writeFailure.indexedDB,
      checkpointFixture(),
      streamIdentityFixture(),
    );

    expect(
      writeFailure.records.has(checkpointStorageKey(streamIdentityFixture())),
    ).toBe(false);

    const readFailure = createIndexedDBTestDouble({ failOpen: true });

    await expect(
      readTimelineCheckpoint(
        readFailure.indexedDB,
        streamIdentityFixture(),
      ),
    ).resolves.toBe(null);
  });
});

describe("timeline checkpoint guard migration", () => {
  it("deletes unsafe v1 checkpoints that do not include stream identity", async () => {
    const { indexedDB, records } = createIndexedDBTestDouble();
    records.set(checkpointStorageKey(streamIdentityFixture()), {
      checkpoint: checkpointFixture(),
      schemaVersion: 1,
      sessionID: RESOLVED_SESSION_UUID,
      storageKey: checkpointStorageKey(streamIdentityFixture()),
    });

    await expect(
      readTimelineCheckpoint(indexedDB, streamIdentityFixture()),
    ).resolves.toBe(null);
    expect(records.has(checkpointStorageKey(streamIdentityFixture()))).toBe(
      false,
    );
  });

  it("deletes legacy session-only checkpoints keyed by alias session id", async () => {
    const { indexedDB, records } = createIndexedDBTestDouble();
    records.set(DEFAULT_FACTORY_SESSION_ID, {
      checkpoint: checkpointFixture(),
      schemaVersion: 2,
      sessionID: DEFAULT_FACTORY_SESSION_ID,
      storageKey: DEFAULT_FACTORY_SESSION_ID,
    });

    await purgeLegacyTimelineCheckpoints(indexedDB);

    expect(records.has(DEFAULT_FACTORY_SESSION_ID)).toBe(false);
  });

  it("rejects alias factory session ids in durable stream identity", async () => {
    const { indexedDB } = createIndexedDBTestDouble();

    await persistTimelineCheckpoint(indexedDB, checkpointFixture(), {
      backendScopeID: "backend-scope-a",
      factorySessionID: DEFAULT_FACTORY_SESSION_ID,
      logicalSessionKeyID: "logical-default",
      streamGenerationID: "2026-06-26T00:00:00Z",
    });

    await expect(
      readTimelineCheckpoint(indexedDB, {
        backendScopeID: "backend-scope-a",
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        logicalSessionKeyID: "logical-default",
        streamGenerationID: "2026-06-26T00:00:00Z",
      }),
    ).resolves.toBe(null);
  });

  it("does not fall back to session-only storage when the stream identity changes", async () => {
    const { indexedDB } = createIndexedDBTestDouble();
    const checkpoint = checkpointFixture();
    const identityA = streamIdentityFixture();
    const identityB = {
      ...identityA,
      streamGenerationID: "2026-06-27T00:00:00Z",
    } satisfies TimelineCheckpointStreamIdentity;

    await persistTimelineCheckpoint(indexedDB, checkpoint, identityA);

    await expect(
      readTimelineCheckpoint(indexedDB, identityB),
    ).resolves.toBe(null);
  });
});

describe("timeline checkpoint reconnect cursors", () => {
  it("builds reconnect cursors only when checkpoint cursor data exists", () => {
    expect(reconnectCursorFromCheckpoint(null)).toBeUndefined();
    expect(
      reconnectCursorFromCheckpoint({
        replayState: emptyReplayWorldState(1),
        selectedTick: 1,
      }),
    ).toBeUndefined();

    expect(reconnectCursorFromCheckpoint(checkpointFixture())).toEqual({
      afterEventId: "event-7",
      afterSequence: 42,
    });
    expect(
      reconnectCursorFromCheckpoint({
        afterSequence: 9,
        replayState: emptyReplayWorldState(9),
        selectedTick: 9,
      }),
    ).toEqual({ afterEventId: undefined, afterSequence: 9 });
  });
});
