import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { emptyReplayWorldState } from "../../timeline/state/timeline/replayWorldStateSupport";
import * as syncPreflightAPI from "../../../api/factory-sessions/sync-preflight";
import {
  bootstrapDashboardSessionSyncPreflight,
  isNonRecoverableSyncPreflightReason,
  streamIdentityFromSyncPreflightResponse,
} from "./dashboard-session-sync-preflight";

const RESOLVED_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";
const STALE_SESSION_UUID = "deadbeef-dead-beef-dead-beefdeadbeef";

function resolvedStreamIdentity() {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: RESOLVED_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

function okPreflightResponse(
  overrides: Partial<syncPreflightAPI.FactorySessionSyncPreflightResponse> = {},
) {
  return {
    backendScopeId: "backend-scope-a",
    checkpointReusable: true,
    factorySessionId: RESOLVED_SESSION_UUID,
    logicalSessionKeyId: "logical-default",
    reasonCode: "ok" as const,
    reconnectCursor: {
      provided: false,
      validForStreamGeneration: true,
    },
    requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
    streamGenerationId: "2026-06-26T00:00:00Z",
    ...overrides,
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

  window.setTimeout(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
  }, 0);

  return request;
}

function installIndexedDBTestDouble() {
  const records = new Map<string, unknown>();
  const database = {
    close: () => {},
    createObjectStore: () => undefined,
    deleteObjectStore: () => undefined,
    objectStoreNames: {
      contains: () => true,
    },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => {
            records.delete(key);
          }),
        get: (key: string) => indexedDBRequest(records.get(key)),
        getAll: () => indexedDBRequest([...records.values()]),
        put: (value: { storageKey: string }) =>
          indexedDBRequest(value.storageKey, () => {
            records.set(value.storageKey, value);
          }),
      }),
    }),
  };
  const indexedDB = {
    open: () => {
      const request = indexedDBRequest(database);
      window.setTimeout(
        () => request.onupgradeneeded?.({} as IDBVersionChangeEvent),
        0,
      );
      return request;
    },
  };

  Object.defineProperty(window, "indexedDB", {
    configurable: true,
    value: indexedDB,
  });

  return records;
}

describe("bootstrapDashboardSessionSyncPreflight recovery", () => {
  let getSyncPreflightSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    installIndexedDBTestDouble();
    getSyncPreflightSpy = vi
      .spyOn(syncPreflightAPI, "getFactorySessionSyncPreflight")
      .mockResolvedValue(okPreflightResponse());
  });

  afterEach(() => {
    getSyncPreflightSpy.mockRestore();
  });

  it("returns recovery for session_not_found when no stored checkpoint can remap", async () => {
    getSyncPreflightSpy.mockResolvedValue({
      checkpointReusable: false,
      reasonCode: "session_not_found",
      reconnectCursor: {
        provided: false,
        validForStreamGeneration: false,
      },
      requestedSessionId: "session-missing",
    });

    await expect(
      bootstrapDashboardSessionSyncPreflight({
        indexedDB: window.indexedDB,
        refreshToken: 0,
        sessionID: "session-missing",
      }),
    ).resolves.toEqual({
      kind: "recovery",
      recovery: {
        reasonCode: "session_not_found",
        requestedSessionId: "session-missing",
      },
    });
  });

  it("remaps an unknown UUID through stored checkpoint logical identity", async () => {
    const records = installIndexedDBTestDouble();
    const identity = resolvedStreamIdentity();
    records.set(
      [
        identity.backendScopeID,
        STALE_SESSION_UUID,
        identity.streamGenerationID,
      ].join("::"),
      {
        checkpoint: {
          afterEventId: "event-7",
          afterSequence: 7,
          replayState: emptyReplayWorldState(7),
          selectedTick: 7,
        },
        schemaVersion: 3,
        storageKey: [
          identity.backendScopeID,
          STALE_SESSION_UUID,
          identity.streamGenerationID,
        ].join("::"),
        streamIdentity: {
          ...identity,
          factorySessionID: STALE_SESSION_UUID,
        },
      },
    );

    getSyncPreflightSpy
      .mockResolvedValueOnce({
        checkpointReusable: false,
        reasonCode: "session_not_found",
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: false,
        },
        requestedSessionId: STALE_SESSION_UUID,
      })
      .mockResolvedValueOnce({
        backendScopeId: identity.backendScopeID,
        checkpointReusable: false,
        factorySessionId: RESOLVED_SESSION_UUID,
        logicalSessionKeyId: identity.logicalSessionKeyID,
        reasonCode: "logical_session_remap",
        reconnectCursor: {
          afterEventId: "event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
        requestedSessionId: STALE_SESSION_UUID,
        streamGenerationId: identity.streamGenerationID,
      });

    await expect(
      bootstrapDashboardSessionSyncPreflight({
        indexedDB: window.indexedDB,
        refreshToken: 0,
        sessionID: STALE_SESSION_UUID,
      }),
    ).resolves.toMatchObject({
      kind: "ready",
      result: {
        checkpoint: null,
        reconnectCursor: undefined,
        remappedFactorySessionId: RESOLVED_SESSION_UUID,
        streamIdentity: identity,
      },
    });
  });
});

describe("bootstrapDashboardSessionSyncPreflight cursor validation", () => {
  let getSyncPreflightSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    installIndexedDBTestDouble();
    getSyncPreflightSpy = vi
      .spyOn(syncPreflightAPI, "getFactorySessionSyncPreflight")
      .mockResolvedValue(okPreflightResponse());
  });

  afterEach(() => {
    getSyncPreflightSpy.mockRestore();
  });

  it("clears checkpoint state when cursor validation is stale", async () => {
    const records = installIndexedDBTestDouble();
    const identity = resolvedStreamIdentity();
    const storageKey = [
      identity.backendScopeID,
      identity.factorySessionID,
      identity.streamGenerationID,
    ].join("::");
    records.set(storageKey, {
      checkpoint: {
        afterEventId: "event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 3,
      storageKey,
      streamIdentity: identity,
    });

    getSyncPreflightSpy
      .mockResolvedValueOnce(okPreflightResponse())
      .mockResolvedValueOnce({
        backendScopeId: identity.backendScopeID,
        checkpointReusable: false,
        factorySessionId: identity.factorySessionID,
        logicalSessionKeyId: identity.logicalSessionKeyID,
        reasonCode: "cursor_stale",
        reconnectCursor: {
          afterEventId: "event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
        requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationId: identity.streamGenerationID,
      });

    await expect(
      bootstrapDashboardSessionSyncPreflight({
        indexedDB: window.indexedDB,
        refreshToken: 0,
        sessionID: DEFAULT_FACTORY_SESSION_ID,
      }),
    ).resolves.toMatchObject({
      kind: "ready",
      result: {
        checkpoint: null,
        reconnectCursor: undefined,
        remappedFactorySessionId: null,
        streamIdentity: identity,
      },
    });
    expect(records.has(storageKey)).toBe(false);
  });
});

describe("bootstrapDashboardSessionSyncPreflight manual refresh", () => {
  let getSyncPreflightSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    installIndexedDBTestDouble();
    getSyncPreflightSpy = vi
      .spyOn(syncPreflightAPI, "getFactorySessionSyncPreflight")
      .mockResolvedValue(okPreflightResponse());
  });

  afterEach(() => {
    getSyncPreflightSpy.mockRestore();
  });

  it("drops persisted checkpoints when the user refreshes a live session tab", async () => {
    const records = installIndexedDBTestDouble();
    const identity = resolvedStreamIdentity();
    const storageKey = [
      identity.backendScopeID,
      identity.factorySessionID,
      identity.streamGenerationID,
    ].join("::");
    records.set(storageKey, {
      checkpoint: {
        afterEventId: "event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 3,
      storageKey,
      streamIdentity: identity,
    });

    await expect(
      bootstrapDashboardSessionSyncPreflight({
        indexedDB: window.indexedDB,
        refreshToken: 1,
        sessionID: DEFAULT_FACTORY_SESSION_ID,
      }),
    ).resolves.toMatchObject({
      kind: "ready",
      result: {
        checkpoint: null,
        reconnectCursor: undefined,
        remappedFactorySessionId: null,
        streamIdentity: identity,
      },
    });
    expect(records.has(storageKey)).toBe(false);
  });
});

describe("dashboard-session-sync-preflight stream identity helpers", () => {
  it("normalizes sync-preflight identity fields for runtime cache keys", () => {
    expect(
      streamIdentityFromSyncPreflightResponse({
        backendScopeId: "backend-scope-a",
        checkpointReusable: true,
        factorySessionId: RESOLVED_SESSION_UUID,
        logicalSessionKeyId: "logical-default",
        reasonCode: "ok",
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: true,
        },
        requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationId: "2026-06-26T00:00:00Z",
      }),
    ).toEqual(resolvedStreamIdentity());
  });

  it("flags unresolved logical-session outcomes as non-recoverable", () => {
    expect(isNonRecoverableSyncPreflightReason("session_not_found")).toBe(true);
    expect(
      isNonRecoverableSyncPreflightReason("logical_session_unresolved"),
    ).toBe(true);
    expect(isNonRecoverableSyncPreflightReason("ok")).toBe(false);
  });
});
