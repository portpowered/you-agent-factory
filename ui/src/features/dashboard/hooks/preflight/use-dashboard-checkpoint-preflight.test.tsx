import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as factorySessionsAPI from "../../../../api/factory-sessions";
import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import * as timelinePublic from "../../../timeline/public";
import * as preflightResolver from "../../lib/preflight/resolve-dashboard-checkpoint-preflight";
import {
  correlationTokenForIdentityScope,
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: bootstrap outcomes share one guarded preflight harness.
describe("useDashboardCheckpointPreflight bootstrap", () => {
  beforeEach(() => {
    resetCheckpointPreflightTestState();
  });

  it("marks preflight ready when timeline checkpoints are disabled", async () => {
    const queryClient = new QueryClient();
    const resolvePreflightSpy = vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
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
    expect(resolvePreflightSpy).not.toHaveBeenCalled();
  });

  it("hydrates checkpoint preflight results from the runner", async () => {
    const queryClient = new QueryClient();
    const restoreCheckpoint = vi.fn();
    vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
    ).mockResolvedValue({
      checkpoint: null,
      checkpointLookupOutcome: "checkpoint_miss",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: false,
      kind: "resume",
      reconnectCursor: {
        afterEventId: "event-2",
        afterSequence: 2,
      },
      requestedSessionId: "session-live-001",
      resolvedSessionId: "session-live-001",
      staleCursorDetected: false,
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
          restoreCheckpoint,
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
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      {
        correlationToken: correlationTokenForIdentityScope({
          backendScopeID: "backend-scope-a",
          factorySessionID: "session-live-001",
          logicalSessionKeyID: "lsk-default",
          streamGenerationID: "generation-1",
        }),
        outcome: "checkpoint_miss",
        recoveryAction: "replay_without_cursor",
      },
    ]);
  });

  it("records confirmed preflight stale-cursor recovery and hands off its safe correlation", async () => {
    const queryClient = new QueryClient();
    const restoreCheckpoint = vi.fn();
    const streamIdentity = {
      backendScopeID: "backend-scope-a",
      factorySessionID: "session-live-001",
      logicalSessionKeyID: "lsk-default",
      streamGenerationID: "generation-1",
    };
    vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
    ).mockResolvedValue({
      checkpoint: null,
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: false,
      kind: "resume",
      requestedSessionId: "session-live-001",
      resolvedSessionId: "session-live-001",
      staleCursorDetected: true,
      streamIdentity,
    });

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "session-live-001::0",
          checkpointsDisabled: false,
          rawSessionID: "session-live-001",
          refreshToken: 0,
          restoreCheckpoint,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    const correlationToken = correlationTokenForIdentityScope(streamIdentity);
    expect(result.current.cursorFreeReplayCorrelationToken).toBe(
      correlationToken,
    );
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      {
        correlationToken,
        outcome: "checkpoint_hit",
        recoveryAction: "reuse_checkpoint",
      },
      {
        correlationToken,
        outcome: "stale_cursor",
        recoveryAction: "invalidate_reconnect_cursor",
      },
    ]);
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
  ])(
    "ignores delayed session A %s after session B becomes active",
    async (outcome) => {
      const race = await startSupersededSessionRace();
      await settleSupersededSessionA(race.sessionAPreflight, outcome);

      expect(race.result.current.resolvedSessionID).toBe(SESSION_B);
      expect(race.result.current.persistedCheckpoint?.selectedTick).toBe(22);
      expect(race.restoreCheckpoint).toHaveBeenCalledTimes(1);
      expect(race.restoreCheckpoint).toHaveBeenCalledWith(
        expect.objectContaining({ factorySessionID: SESSION_B }),
        expect.objectContaining({ selectedTick: 22 }),
      );
      expect(race.readCheckpointSpy).toHaveBeenCalledTimes(1);
      expect(race.readCheckpointSpy).toHaveBeenCalledWith(
        window.indexedDB,
        expect.objectContaining({ factorySessionID: SESSION_B }),
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
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
      expect(readSessionPersistenceInvalidationRecords()).toEqual([
        {
          correlationToken: correlationTokenForIdentityScope({
            backendScopeID: `backend-${SESSION_B}`,
            factorySessionID: SESSION_B,
            logicalSessionKeyID: `logical-${SESSION_B}`,
            streamGenerationID: `generation-${SESSION_B}`,
          }),
          outcome: "checkpoint_hit",
          recoveryAction: "reuse_checkpoint",
        },
      ]);
    },
  );
});
