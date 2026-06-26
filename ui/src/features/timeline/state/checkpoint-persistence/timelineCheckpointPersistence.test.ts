import { describe, expect, it, vi } from "vitest";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
} from "../timelineCheckpointPersistence";

interface StoredCheckpointEnvelope {
  checkpoint?: {
    afterEventId?: string;
    afterSequence?: number;
    replayState?: ReturnType<typeof emptyReplayWorldState>;
    selectedTick: number;
  };
  schemaVersion: number;
  sessionID: string;
}

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
            : indexedDBRequest(value.sessionID, () => {
                records.set(value.sessionID, value);
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
  };
}

describe("timeline checkpoint persistence", () => {
  it("persists compact checkpoints without mutating the live replay state", async () => {
    const { indexedDB } = createIndexedDBTestDouble();
    const checkpoint = checkpointFixture();

    await persistTimelineCheckpoint(indexedDB, "session-a", checkpoint);
    const restored = await readTimelineCheckpoint(indexedDB, "session-a");

    expect(checkpoint.replayState.textBlobsByID.long).toHaveLength(600);
    expect(restored?.selectedTick).toBe(7);
    expect(restored?.afterEventId).toBe("event-7");
    expect(restored?.afterSequence).toBe(42);
    expect(restored?.replayState.textBlobsByID.short).toBe("kept");
    expect(restored?.replayState.textBlobsByID.long).toContain(
      "[checkpoint truncated 88 chars]",
    );
    expect(restored?.replayState.textBlobsByID.long.length).toBeLessThan(600);
  });

  it("drops invalid stored checkpoints and ignores missing persistence inputs", async () => {
    const { indexedDB, records } = createIndexedDBTestDouble();
    records.set("session-a", {
      checkpoint: {
        replayState: emptyReplayWorldState(1),
        selectedTick: 1,
      },
      schemaVersion: 999,
      sessionID: "session-a",
    });

    await expect(readTimelineCheckpoint(undefined, "session-a")).resolves.toBe(
      null,
    );
    await expect(readTimelineCheckpoint(indexedDB, null)).resolves.toBe(null);
    await persistTimelineCheckpoint(indexedDB, "session-a", undefined);

    await expect(readTimelineCheckpoint(indexedDB, "session-a")).resolves.toBe(
      null,
    );
    expect(records.has("session-a")).toBe(false);
  });

  it("creates the checkpoint store during database upgrades", async () => {
    const { createObjectStore, indexedDB } = createIndexedDBTestDouble({
      storeExists: false,
    });

    await persistTimelineCheckpoint(
      indexedDB,
      "session-a",
      checkpointFixture(),
    );

    expect(createObjectStore).toHaveBeenCalledWith("checkpoints", {
      keyPath: "sessionID",
    });
  });

  it("cleans stale checkpoint data when IndexedDB operations fail", async () => {
    const writeFailure = createIndexedDBTestDouble({ failPut: true });
    writeFailure.records.set("session-a", {
      checkpoint: {
        replayState: emptyReplayWorldState(1),
        selectedTick: 1,
      },
      schemaVersion: 1,
      sessionID: "session-a",
    });

    await persistTimelineCheckpoint(
      writeFailure.indexedDB,
      "session-a",
      checkpointFixture(),
    );

    expect(writeFailure.records.has("session-a")).toBe(false);

    const readFailure = createIndexedDBTestDouble({ failOpen: true });

    await expect(
      readTimelineCheckpoint(readFailure.indexedDB, "session-a"),
    ).resolves.toBe(null);
  });

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
