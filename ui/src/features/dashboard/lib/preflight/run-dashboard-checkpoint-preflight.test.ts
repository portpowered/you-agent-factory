import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as factorySessionsAPI from "../../../../api/factory-sessions";
import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import * as timelinePublic from "../../../timeline/public";
import {
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../session-persistence/diagnostics";
import { runDashboardCheckpointPreflight } from "./run-dashboard-checkpoint-preflight";

const RESOLVED_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function storedSessionRecord(sessionID: string, selectedTick: number) {
  return {
    checkpoint: { replayState: {}, selectedTick },
    schemaVersion: 3,
    storageKey: `checkpoint-${sessionID}`,
    streamIdentity: {
      backendScopeID: `backend-${sessionID}`,
      factorySessionID: sessionID,
      logicalSessionKeyID: `logical-${sessionID}`,
      streamGenerationID: `generation-${sessionID}`,
    },
  };
}

function buildPreflightResponse(
  overrides: Partial<FactorySessionSyncPreflightResponse> = {},
): FactorySessionSyncPreflightResponse {
  return {
    backendScopeId: "backend-scope-a",
    checkpointReusable: true,
    factorySessionId: RESOLVED_SESSION_UUID,
    logicalSessionKeyId: "lsk-default",
    reasonCode: FactorySessionSyncPreflightReasonCode.ok,
    reconnectCursor: {
      afterEventId: "event-3",
      afterSequence: 3,
      provided: true,
      validForStreamGeneration: true,
    },
    requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
    streamGenerationId: "generation-1",
    ...overrides,
  };
}

function installPreflightMocks() {
  const queryClient = {
    removeQueries: vi.fn(),
  };
  const getSyncPreflightSpy = vi
    .spyOn(factorySessionsAPI, "getFactorySessionSyncPreflight")
    .mockResolvedValue(buildPreflightResponse());
  const peekCheckpointSpy = vi
    .spyOn(timelinePublic, "peekPersistedTimelineCheckpoint")
    .mockResolvedValue(null);
  const clearCheckpointsSpy = vi
    .spyOn(timelinePublic, "clearTimelineCheckpointsForSession")
    .mockResolvedValue(undefined);
  const clearCheckpointSpy = vi
    .spyOn(timelinePublic, "deletePersistedTimelineCheckpoint")
    .mockResolvedValue(undefined);
  const readCheckpointSpy = vi
    .spyOn(timelinePublic, "readTimelineCheckpoint")
    .mockResolvedValue(null);

  return {
    clearCheckpointsSpy,
    clearCheckpointSpy,
    getSyncPreflightSpy,
    peekCheckpointSpy,
    queryClient,
    readCheckpointSpy,
  };
}

describe("runDashboardCheckpointPreflight alias remap", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("keeps the default alias in session store while resolving runtime UUID streams", async () => {
    const { queryClient } = installPreflightMocks();
    const onRemapSessionID = vi.fn();

    const hydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID,
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: DEFAULT_FACTORY_SESSION_ID,
      restoreCheckpoint: vi.fn(),
    });

    expect(onRemapSessionID).not.toHaveBeenCalled();
    expect(hydration.resolvedSessionID).toBe(RESOLVED_SESSION_UUID);
    expect(hydration.streamIdentity).toEqual({
      backendScopeID: "backend-scope-a",
      factorySessionID: RESOLVED_SESSION_UUID,
      logicalSessionKeyID: "lsk-default",
      streamGenerationID: "generation-1",
    });
  });

  it("sends logical identity hints from envelope stream identity when checkpoint sync identity is absent", async () => {
    const { getSyncPreflightSpy, queryClient } = installPreflightMocks();
    const staleSessionID = "session-stale-001";
    const remappedSessionID = "session-remapped-002";
    const logicalSessionKeyID = "lsk-named-target";
    vi.spyOn(
      timelinePublic,
      "peekPersistedTimelineCheckpoint",
    ).mockResolvedValue({
      checkpoint: {
        afterEventId: "event-7",
        afterSequence: 7,
        replayState: {} as never,
        selectedTick: 7,
      },
      streamIdentity: {
        backendScopeID: "backend-scope-a",
        factorySessionID: staleSessionID,
        logicalSessionKeyID,
        streamGenerationID: "generation-stale",
      },
    });
    getSyncPreflightSpy.mockResolvedValue(
      buildPreflightResponse({
        checkpointReusable: false,
        factorySessionId: remappedSessionID,
        logicalSessionKeyId: logicalSessionKeyID,
        reasonCode: FactorySessionSyncPreflightReasonCode.logical_session_remap,
        reconnectCursor: {
          afterEventId: "event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
        requestedSessionId: staleSessionID,
        streamGenerationId: "generation-live",
      }),
    );
    const operationOrder: string[] = [];
    const onRemapSessionID = vi.fn(() => operationOrder.push("remap"));
    const restoreCheckpoint = vi.fn(() => operationOrder.push("restore"));

    const hydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID,
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: staleSessionID,
      restoreCheckpoint,
    });

    expect(getSyncPreflightSpy).toHaveBeenCalledWith(
      staleSessionID,
      {
        afterEventId: "event-7",
        afterSequence: 7,
      },
      {
        backendScopeId: "backend-scope-a",
        logicalSessionKeyId: logicalSessionKeyID,
      },
    );
    expect(onRemapSessionID).toHaveBeenCalledWith(remappedSessionID);
    expect(
      timelinePublic.deletePersistedTimelineCheckpoint,
    ).toHaveBeenCalledWith(
      window.indexedDB,
      expect.objectContaining({
        streamIdentity: expect.objectContaining({
          factorySessionID: staleSessionID,
          logicalSessionKeyID,
          streamGenerationID: "generation-stale",
        }),
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(
      timelinePublic.clearTimelineCheckpointsForSession,
    ).not.toHaveBeenCalled();
    expect(restoreCheckpoint).not.toHaveBeenCalled();
    expect(operationOrder).toEqual(["remap"]);
    expect(hydration.initialReconnectCursor).toBeUndefined();
    expect(hydration.resolvedSessionID).toBe(remappedSessionID);
  });
});

describe("runDashboardCheckpointPreflight selected identity remap", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("remaps the selected session id for logical session replacement", async () => {
    const { getSyncPreflightSpy, queryClient } = installPreflightMocks();
    const onRemapSessionID = vi.fn();
    getSyncPreflightSpy.mockResolvedValue(
      buildPreflightResponse({
        checkpointReusable: false,
        factorySessionId: "session-remapped-002",
        reasonCode: FactorySessionSyncPreflightReasonCode.logical_session_remap,
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: false,
        },
        requestedSessionId: "session-stale-001",
      }),
    );

    const hydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID,
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: "session-stale-001",
      restoreCheckpoint: vi.fn(),
    });

    expect(onRemapSessionID).toHaveBeenCalledWith("session-remapped-002");
    expect(hydration.initialReconnectCursor).toBeUndefined();
    expect(hydration.resolvedSessionID).toBe("session-remapped-002");
  });
});

describe("runDashboardCheckpointPreflight recovery", () => {
  let getSyncPreflightSpy: ReturnType<typeof vi.spyOn>;
  let peekCheckpointSpy: ReturnType<typeof vi.spyOn>;
  let clearCheckpointsSpy: ReturnType<typeof vi.spyOn>;
  let clearCheckpointSpy: ReturnType<typeof vi.spyOn>;
  let readCheckpointSpy: ReturnType<typeof vi.spyOn>;
  let queryClient: ReturnType<typeof installPreflightMocks>["queryClient"];

  beforeEach(() => {
    ({
      clearCheckpointsSpy,
      clearCheckpointSpy,
      getSyncPreflightSpy,
      peekCheckpointSpy,
      queryClient,
      readCheckpointSpy,
    } = installPreflightMocks());
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("drops reconnect cursors when persisted stream identity does not match preflight", async () => {
    peekCheckpointSpy.mockResolvedValue({
      checkpoint: {
        afterEventId: "event-1",
        afterSequence: 1,
        syncIdentity: {
          backendScopeId: "backend-scope-a",
          factorySessionId: RESOLVED_SESSION_UUID,
          logicalSessionKeyId: "lsk-default",
          streamGenerationId: "generation-1",
        },
        worldState: {} as never,
      },
      streamIdentity: {
        backendScopeID: "backend-scope-a",
        factorySessionID: RESOLVED_SESSION_UUID,
        logicalSessionKeyID: "lsk-default",
        streamGenerationID: "generation-stale",
      },
    });

    const hydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID: vi.fn(),
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: RESOLVED_SESSION_UUID,
      restoreCheckpoint: vi.fn(),
    });

    expect(clearCheckpointsSpy).not.toHaveBeenCalled();
    expect(clearCheckpointSpy).toHaveBeenCalledWith(
      window.indexedDB,
      expect.objectContaining({
        streamIdentity: expect.objectContaining({
          streamGenerationID: "generation-stale",
        }),
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    expect(hydration.initialReconnectCursor).toBeUndefined();
  });

  it("returns typed recovery for unresolved logical targets", async () => {
    getSyncPreflightSpy.mockResolvedValue(
      buildPreflightResponse({
        checkpointReusable: false,
        reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: false,
        },
        requestedSessionId: "session-missing",
      }),
    );

    const hydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID: vi.fn(),
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: "session-missing",
      restoreCheckpoint: vi.fn(),
    });

    expect(hydration.preflightRecovery).toEqual({
      reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
      requestedSessionId: "session-missing",
    });
    expect(hydration.resolvedSessionID).toBeNull();
    expect(clearCheckpointsSpy).toHaveBeenCalled();
  });

  it("restores persisted checkpoints when preflight reuses reconnect cursors", async () => {
    const restoreCheckpoint = vi.fn();
    const checkpoint = {
      afterEventId: "event-3",
      afterSequence: 3,
      syncIdentity: {
        backendScopeId: "backend-scope-a",
        factorySessionId: RESOLVED_SESSION_UUID,
        logicalSessionKeyId: "lsk-default",
        streamGenerationId: "generation-1",
      },
      worldState: {} as never,
    };
    readCheckpointSpy.mockResolvedValue(checkpoint);

    const hydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID: vi.fn(),
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: RESOLVED_SESSION_UUID,
      restoreCheckpoint,
    });

    expect(restoreCheckpoint).toHaveBeenCalledWith(
      expect.objectContaining({
        afterEventId: "event-3",
        syncIdentity: expect.objectContaining({
          factorySessionId: RESOLVED_SESSION_UUID,
          logicalSessionKeyId: "lsk-default",
        }),
      }),
    );
    expect(hydration.persistedCheckpoint).toEqual(checkpoint);
    expect(hydration.initialReconnectCursor).toEqual({
      afterEventId: "event-3",
      afterSequence: 3,
    });
  });
});

describe("runDashboardCheckpointPreflight persistence cancellation", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    resetSessionPersistenceInvalidationRecords();
  });

  it("aborts a held stale-session delete after the active session hydrates", async () => {
    const sessionA = "session-a";
    const sessionB = "session-b";
    const { controls, indexedDB, records } =
      createControlledIndexedDBTestDouble<{
        checkpoint: { replayState: object; selectedTick: number };
        schemaVersion: number;
        storageKey: string;
        streamIdentity: {
          backendScopeID: string;
          factorySessionID: string;
          logicalSessionKeyID: string;
          streamGenerationID: string;
        };
      }>();
    records.set(`checkpoint-${sessionA}`, storedSessionRecord(sessionA, 11));
    records.set(`checkpoint-${sessionB}`, storedSessionRecord(sessionB, 22));
    vi.stubGlobal("indexedDB", indexedDB);
    vi.spyOn(
      timelinePublic,
      "peekPersistedTimelineCheckpoint",
    ).mockResolvedValue(null);
    vi.spyOn(timelinePublic, "readTimelineCheckpoint").mockResolvedValue(null);
    vi.spyOn(
      factorySessionsAPI,
      "getFactorySessionSyncPreflight",
    ).mockImplementation(async (sessionID) =>
      sessionID === sessionA
        ? buildPreflightResponse({
            checkpointReusable: false,
            factorySessionId: sessionA,
            reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
            reconnectCursor: {
              provided: false,
              validForStreamGeneration: false,
            },
            requestedSessionId: sessionA,
          })
        : buildPreflightResponse({
            factorySessionId: sessionB,
            requestedSessionId: sessionB,
          }),
    );

    const queryClient = new QueryClient();
    queryClient.setQueryData(["session-race", sessionA], "cache-a");
    queryClient.setQueryData(["session-race", sessionB], "cache-b");
    const removeQueries = vi.spyOn(queryClient, "removeQueries");
    const restoreCheckpoint = vi.fn();
    const onRemapSessionID = vi.fn();
    const onStreamOffline = vi.fn();
    const abortA = new AbortController();
    let sessionACurrent = true;
    const sessionAPreflight = runDashboardCheckpointPreflight({
      isCurrent: () => sessionACurrent,
      onRemapSessionID,
      onStreamOffline,
      queryClient,
      rawSessionID: sessionA,
      restoreCheckpoint,
      signal: abortA.signal,
    });

    await flushPromiseContinuations();
    await flushPromiseContinuations();
    controls.succeed("open");
    await flushPromiseContinuations();
    controls.succeed("getAll");
    await flushPromiseContinuations();
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["delete"]);

    const sessionBHydration = await runDashboardCheckpointPreflight({
      isCurrent: () => true,
      onRemapSessionID,
      onStreamOffline,
      queryClient,
      rawSessionID: sessionB,
      restoreCheckpoint,
    });
    expect(sessionBHydration.resolvedSessionID).toBe(sessionB);

    sessionACurrent = false;
    abortA.abort();
    controls.succeed("delete");
    await sessionAPreflight;

    expect(records.get(`checkpoint-${sessionA}`)?.checkpoint.selectedTick).toBe(
      11,
    );
    expect(records.get(`checkpoint-${sessionB}`)?.checkpoint.selectedTick).toBe(
      22,
    );
    expect(queryClient.getQueryData(["session-race", sessionA])).toBe(
      "cache-a",
    );
    expect(queryClient.getQueryData(["session-race", sessionB])).toBe(
      "cache-b",
    );
    expect(removeQueries).not.toHaveBeenCalled();
    expect(restoreCheckpoint).not.toHaveBeenCalled();
    expect(onRemapSessionID).not.toHaveBeenCalled();
    expect(onStreamOffline).not.toHaveBeenCalled();
    expect(readSessionPersistenceInvalidationRecords()).toEqual([]);
  });
});
