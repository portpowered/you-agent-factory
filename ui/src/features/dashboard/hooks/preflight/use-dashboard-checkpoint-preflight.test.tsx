import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as factorySessionsAPI from "../../../../api/factory-sessions";
import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import * as timelinePublic from "../../../timeline/public";
import * as preflightRunner from "../../lib/preflight/run-dashboard-checkpoint-preflight";
import {
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../../lib/session-persistence/diagnostics";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
import { useDashboardCheckpointPreflight } from "./use-dashboard-checkpoint-preflight";

const { remapSelectedSessionID } = vi.hoisted(() => ({
  remapSelectedSessionID: vi.fn(),
}));

vi.mock("../../session/dashboard-session-provider", () => ({
  useRemapDashboardSelectedSession: () => remapSelectedSessionID,
}));

interface Deferred<T> {
  promise: Promise<T>;
  reject: (reason?: unknown) => void;
  resolve: (value: T) => void;
}

function createDeferred<T>(): Deferred<T> {
  let reject!: Deferred<T>["reject"];
  let resolve!: Deferred<T>["resolve"];
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    reject = rejectPromise;
    resolve = resolvePromise;
  });
  return { promise, reject, resolve };
}

function buildPreflightResponse({
  reasonCode = FactorySessionSyncPreflightReasonCode.ok,
  requestedSessionId,
  resolvedSessionId = requestedSessionId,
}: {
  reasonCode?: FactorySessionSyncPreflightResponse["reasonCode"];
  requestedSessionId: string;
  resolvedSessionId?: string;
}): FactorySessionSyncPreflightResponse {
  const reusable = reasonCode === FactorySessionSyncPreflightReasonCode.ok;
  return {
    backendScopeId: `backend-${resolvedSessionId}`,
    checkpointReusable: reusable,
    factorySessionId: resolvedSessionId,
    logicalSessionKeyId: `logical-${resolvedSessionId}`,
    reasonCode,
    reconnectCursor: reusable
      ? {
          afterEventId: `event-${resolvedSessionId}`,
          afterSequence: 2,
          provided: true,
          validForStreamGeneration: true,
        }
      : {
          provided: false,
          validForStreamGeneration: false,
        },
    requestedSessionId,
    streamGenerationId: `generation-${resolvedSessionId}`,
  };
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function resetCheckpointPreflightTestState(): void {
  vi.restoreAllMocks();
  remapSelectedSessionID.mockReset();
  resetSessionPersistenceInvalidationRecords();
  useDashboardStreamStore.setState({
    setStreamState: useDashboardStreamStore.getState().setStreamState,
    resetStreamState: useDashboardStreamStore.getState().resetStreamState,
  });
}

describe("useDashboardCheckpointPreflight bootstrap", () => {
  beforeEach(() => {
    resetCheckpointPreflightTestState();
  });

  it("marks preflight ready when timeline checkpoints are disabled", async () => {
    const queryClient = new QueryClient();
    const runPreflightSpy = vi.spyOn(
      preflightRunner,
      "runDashboardCheckpointPreflight",
    );

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "~default::0",
          checkpointsDisabled: true,
          rawSessionID: "~default",
          refreshToken: 0,
          restoreCheckpoint: vi.fn(),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(result.current.preflightReady).toBe(true);
    });
    expect(result.current.resolvedSessionID).toBe("~default");
    expect(runPreflightSpy).not.toHaveBeenCalled();
  });

  it("hydrates checkpoint preflight results from the runner", async () => {
    const queryClient = new QueryClient();
    vi.spyOn(
      preflightRunner,
      "runDashboardCheckpointPreflight",
    ).mockResolvedValue({
      initialReconnectCursor: {
        afterEventId: "event-2",
        afterSequence: 2,
      },
      persistedCheckpoint: null,
      preflightError: null,
      preflightRecovery: null,
      resolvedSessionID: "session-live-001",
      streamIdentity: {
        backendScopeID: "backend-scope-a",
        factorySessionID: "session-live-001",
        logicalSessionKeyID: "lsk-default",
        streamGenerationID: "generation-1",
      },
    });

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "session-live-001::0",
          checkpointsDisabled: false,
          rawSessionID: "session-live-001",
          refreshToken: 0,
          restoreCheckpoint: vi.fn(),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(result.current.preflightReady).toBe(true);
    });
    expect(result.current.resolvedSessionID).toBe("session-live-001");
    expect(result.current.initialReconnectCursor).toEqual({
      afterEventId: "event-2",
      afterSequence: 2,
    });
  });
});

const SESSION_A = "session-a";
const SESSION_B = "session-b";

function installSessionRacePersistenceMocks() {
  vi.spyOn(
    timelinePublic,
    "peekPersistedTimelineCheckpoint",
  ).mockImplementation(async (_indexedDB, sessionID) => ({
    checkpoint: {
      afterEventId: `event-${sessionID}`,
      afterSequence: 2,
      replayState: {} as never,
      selectedTick: sessionID === SESSION_A ? 11 : 22,
    },
    storageKey: `checkpoint-${sessionID}`,
    streamIdentity: {
      backendScopeID: `backend-${sessionID}`,
      factorySessionID: sessionID ?? "",
      logicalSessionKeyID: `logical-${sessionID}`,
      streamGenerationID: `generation-${sessionID}`,
    },
  }));
  const readCheckpointSpy = vi
    .spyOn(timelinePublic, "readTimelineCheckpoint")
    .mockImplementation(async (_indexedDB, identity) => ({
      afterEventId: `event-${identity?.factorySessionID}`,
      afterSequence: 2,
      replayState: {} as never,
      selectedTick: identity?.factorySessionID === SESSION_A ? 11 : 22,
    }));
  const clearCheckpointsSpy = vi
    .spyOn(timelinePublic, "clearTimelineCheckpointsForSession")
    .mockResolvedValue(undefined);
  return { clearCheckpointsSpy, readCheckpointSpy };
}

async function startSupersededSessionRace() {
  const sessionAPreflight =
    createDeferred<FactorySessionSyncPreflightResponse>();
  const sessionBPreflight =
    createDeferred<FactorySessionSyncPreflightResponse>();
  const queryClient = new QueryClient();
  queryClient.setQueryData(["session-race", SESSION_A], "cache-a");
  queryClient.setQueryData(["session-race", SESSION_B], "cache-b");
  const removeQueriesSpy = vi.spyOn(queryClient, "removeQueries");
  const restoreCheckpoint = vi.fn();
  const setStreamState = vi.fn();
  useDashboardStreamStore.setState({ setStreamState });
  const persistenceSpies = installSessionRacePersistenceMocks();
  vi.spyOn(
    factorySessionsAPI,
    "getFactorySessionSyncPreflight",
  ).mockImplementation(async (sessionID) =>
    sessionID === SESSION_A
      ? sessionAPreflight.promise
      : sessionBPreflight.promise,
  );

  const hook = renderHook(
    ({ sessionID }) =>
      useDashboardCheckpointPreflight({
        checkpointHydrationKey: `${sessionID}::0`,
        checkpointsDisabled: false,
        rawSessionID: sessionID,
        refreshToken: 0,
        restoreCheckpoint,
      }),
    {
      initialProps: { sessionID: SESSION_A },
      wrapper: createWrapper(queryClient),
    },
  );
  await waitFor(() => {
    expect(
      factorySessionsAPI.getFactorySessionSyncPreflight,
    ).toHaveBeenCalledWith(SESSION_A, expect.anything(), expect.anything());
  });
  hook.rerender({ sessionID: SESSION_B });
  await waitFor(() => {
    expect(
      factorySessionsAPI.getFactorySessionSyncPreflight,
    ).toHaveBeenCalledWith(SESSION_B, expect.anything(), expect.anything());
  });
  await act(async () => {
    sessionBPreflight.resolve(
      buildPreflightResponse({ requestedSessionId: SESSION_B }),
    );
  });
  await waitFor(() => {
    expect(hook.result.current.resolvedSessionID).toBe(SESSION_B);
  });

  return {
    ...hook,
    ...persistenceSpies,
    queryClient,
    removeQueriesSpy,
    restoreCheckpoint,
    sessionAPreflight,
    setStreamState,
  };
}

type SessionAOutcome =
  | "success"
  | "logical remap"
  | "offline failure"
  | "non-recoverable invalidation"
  | "stale cursor invalidation"
  | "stream identity invalidation";

async function settleSupersededSessionA(
  deferred: Deferred<FactorySessionSyncPreflightResponse>,
  outcome: SessionAOutcome,
): Promise<void> {
  await act(async () => {
    if (outcome === "offline failure") {
      deferred.reject(new Error("session A offline"));
    } else {
      const response = buildPreflightResponse({
        reasonCode:
          outcome === "logical remap"
            ? FactorySessionSyncPreflightReasonCode.logical_session_remap
            : outcome === "non-recoverable invalidation"
              ? FactorySessionSyncPreflightReasonCode.session_not_found
              : outcome === "stale cursor invalidation"
                ? FactorySessionSyncPreflightReasonCode.cursor_stale
                : FactorySessionSyncPreflightReasonCode.ok,
        requestedSessionId: SESSION_A,
        resolvedSessionId:
          outcome === "logical remap" ? "session-a-remapped" : SESSION_A,
      });
      if (outcome === "stream identity invalidation") {
        response.streamGenerationId = "generation-session-a-replacement";
      }
      deferred.resolve(response);
    }
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("useDashboardCheckpointPreflight superseded session races", () => {
  beforeEach(resetCheckpointPreflightTestState);

  it.each<SessionAOutcome>([
    "success",
    "logical remap",
    "offline failure",
    "non-recoverable invalidation",
    "stale cursor invalidation",
    "stream identity invalidation",
  ])("ignores delayed session A %s after session B becomes active", async (outcome) => {
    const race = await startSupersededSessionRace();
    await settleSupersededSessionA(race.sessionAPreflight, outcome);

    expect(race.result.current.resolvedSessionID).toBe(SESSION_B);
    expect(race.result.current.persistedCheckpoint?.selectedTick).toBe(22);
    expect(race.restoreCheckpoint).toHaveBeenCalledTimes(1);
    expect(race.restoreCheckpoint).toHaveBeenCalledWith(
      expect.objectContaining({ selectedTick: 22 }),
    );
    expect(race.readCheckpointSpy).toHaveBeenCalledTimes(1);
    expect(race.readCheckpointSpy).toHaveBeenCalledWith(
      window.indexedDB,
      expect.objectContaining({ factorySessionID: SESSION_B }),
    );
    expect(remapSelectedSessionID).not.toHaveBeenCalled();
    expect(race.setStreamState).not.toHaveBeenCalled();
    expect(race.clearCheckpointsSpy).not.toHaveBeenCalled();
    expect(race.removeQueriesSpy).not.toHaveBeenCalled();
    expect(race.queryClient.getQueryData(["session-race", SESSION_A])).toBe(
      "cache-a",
    );
    expect(race.queryClient.getQueryData(["session-race", SESSION_B])).toBe(
      "cache-b",
    );
    expect(readSessionPersistenceInvalidationRecords()).toEqual([]);
  });
});

describe("useDashboardCheckpointPreflight recovery and errors", () => {
  beforeEach(() => {
    resetCheckpointPreflightTestState();
  });

  it("surfaces non-recoverable preflight recovery without stream identity", async () => {
    const queryClient = new QueryClient();
    vi.spyOn(
      preflightRunner,
      "runDashboardCheckpointPreflight",
    ).mockResolvedValue({
      initialReconnectCursor: undefined,
      persistedCheckpoint: null,
      preflightError: null,
      preflightRecovery: {
        reasonCode: "session_not_found",
        requestedSessionId: "missing-session",
      },
      resolvedSessionID: null,
      streamIdentity: null,
    });

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "missing-session::0",
          checkpointsDisabled: false,
          rawSessionID: "missing-session",
          refreshToken: 0,
          restoreCheckpoint: vi.fn(),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(result.current.preflightReady).toBe(true);
    });
    expect(result.current.preflightRecovery).toEqual({
      reasonCode: "session_not_found",
      requestedSessionId: "missing-session",
    });
    expect(result.current.resolvedSessionID).toBeNull();
    expect(result.current.streamIdentity).toBeNull();
  });

  it("hydrates checkpoint state on preflight error without marking ready", async () => {
    const queryClient = new QueryClient();
    vi.spyOn(
      preflightRunner,
      "runDashboardCheckpointPreflight",
    ).mockResolvedValue({
      initialReconnectCursor: undefined,
      persistedCheckpoint: null,
      preflightError: new Error("validation failed"),
      preflightRecovery: null,
      resolvedSessionID: null,
      streamIdentity: null,
    });

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "session-live-001::0",
          checkpointsDisabled: false,
          rawSessionID: "session-live-001",
          refreshToken: 0,
          restoreCheckpoint: vi.fn(),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(result.current.checkpointHydrated).toBe(true);
    });
    expect(result.current.preflightReady).toBe(false);
    expect(result.current.preflightError?.message).toBe("validation failed");
  });

  it("marks the stream offline when checkpoint preflight rejects", async () => {
    const queryClient = new QueryClient();
    const setStreamState = vi.fn();
    useDashboardStreamStore.setState({ setStreamState });
    vi.spyOn(
      timelinePublic,
      "peekPersistedTimelineCheckpoint",
    ).mockResolvedValue(null);
    vi.spyOn(
      timelinePublic,
      "clearTimelineCheckpointsForSession",
    ).mockResolvedValue(undefined);
    vi.spyOn(
      factorySessionsAPI,
      "getFactorySessionSyncPreflight",
    ).mockRejectedValue(new Error("network down"));

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "session-live-001::0",
          checkpointsDisabled: false,
          rawSessionID: "session-live-001",
          refreshToken: 0,
          restoreCheckpoint: vi.fn(),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(result.current.preflightError?.message).toBe("network down");
    });
    expect(setStreamState).toHaveBeenCalledWith({
      message: "network down",
      status: "offline",
    });
    expect(result.current.checkpointHydrated).toBe(true);
    expect(result.current.preflightReady).toBe(false);
  });
});
