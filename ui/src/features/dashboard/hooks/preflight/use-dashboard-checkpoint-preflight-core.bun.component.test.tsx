import { beforeEach, describe, expect, it, mock } from "bun:test";
import { QueryClient } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";

import type { TimelineCheckpointStreamIdentity } from "../../../timeline/state/timelineCheckpointPersistence";
import type { FactoryTimelineCheckpoint } from "../../../timeline/state/timeline/storeState";
import type { DashboardCheckpointPreflightResolution } from "../../lib/preflight/resolve-dashboard-checkpoint-preflight";
import {
  correlationTokenForIdentityScope,
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../../lib/session-persistence/diagnostics";
import {
  type DashboardCheckpointPreflightEffects,
  useDashboardCheckpointPreflightCore,
} from "./use-dashboard-checkpoint-preflight";

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function streamIdentity(
  factorySessionID: string,
): TimelineCheckpointStreamIdentity {
  return {
    backendScopeID: `backend-${factorySessionID}`,
    factorySessionID,
    logicalSessionKeyID: `logical-${factorySessionID}`,
    streamGenerationID: `generation-${factorySessionID}`,
  };
}

function checkpoint(selectedTick: number): FactoryTimelineCheckpoint {
  return {
    afterEventId: `event-${selectedTick}`,
    afterSequence: selectedTick,
    materializedWorkOutcomeState: {},
    replayState: {},
    selectedTick,
  } as FactoryTimelineCheckpoint;
}

function resumeResolution(
  requestedSessionId: string,
  options: {
    checkpoint?: FactoryTimelineCheckpoint | null;
    staleCursorDetected?: boolean;
  } = {},
): DashboardCheckpointPreflightResolution {
  return {
    checkpoint: options.checkpoint ?? null,
    checkpointLookupOutcome: options.checkpoint
      ? "checkpoint_hit"
      : "checkpoint_miss",
    checkpointToDelete: null,
    clearRequestedSessionCheckpoint: false,
    kind: "resume",
    reconnectCursor: {
      afterEventId: `event-${requestedSessionId}`,
      afterSequence: 2,
    },
    requestedSessionId,
    resolvedSessionId: requestedSessionId,
    staleCursorDetected: options.staleCursorDetected ?? false,
    streamIdentity: streamIdentity(requestedSessionId),
  };
}

function createHarness(
  resolve: DashboardCheckpointPreflightEffects["resolvePreflight"] = async ({
    requestedSessionId,
  }) => resumeResolution(requestedSessionId),
) {
  const clearCheckpointsForSession = mock(async () => {});
  const deleteCheckpoint = mock(async () => {});
  const recoverSessionState = mock(() => {});
  const resolvePreflight = mock(resolve);
  const effects = {
    clearCheckpointsForSession,
    deleteCheckpoint,
    recoverSessionState,
    resolvePreflight,
  } satisfies DashboardCheckpointPreflightEffects;
  return {
    effects,
    queryClient: new QueryClient(),
    remapSelectedSessionID: mock(() => {}),
    restoreCheckpoint: mock(() => {}),
    setStreamState: mock(() => {}),
  };
}

interface RenderProps {
  checkpointsDisabled: boolean;
  sessionID: string;
}

function renderPreflightCore(
  harness: ReturnType<typeof createHarness>,
  initialProps: RenderProps,
) {
  return renderHook(
    ({ checkpointsDisabled, sessionID }: RenderProps) =>
      useDashboardCheckpointPreflightCore({
        checkpointHydrationKey: `${sessionID}::0`,
        checkpointsDisabled,
        effects: harness.effects,
        queryClient: harness.queryClient,
        rawSessionID: sessionID,
        refreshToken: 0,
        remapSelectedSessionID: harness.remapSelectedSessionID,
        restoreCheckpoint: harness.restoreCheckpoint,
        setStreamState: harness.setStreamState,
      }),
    { initialProps },
  );
}

beforeEach(() => {
  resetSessionPersistenceInvalidationRecords();
});

describe("dashboard checkpoint preflight core bootstrap", () => {
  it("becomes ready without resolving when checkpoints are disabled", async () => {
    const harness = createHarness();
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: true,
      sessionID: "~default",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(result.current.checkpointHydrated).toBe(true);
    expect(result.current.resolvedSessionID).toBe("~default");
    expect(harness.effects.resolvePreflight).not.toHaveBeenCalled();
  });

  it("hydrates a reusable checkpoint and reconnect cursor", async () => {
    const persisted = checkpoint(12);
    const harness = createHarness(async ({ requestedSessionId }) =>
      resumeResolution(requestedSessionId, { checkpoint: persisted }),
    );
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "session-live",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(result.current.persistedCheckpoint).toBe(persisted);
    expect(result.current.initialReconnectCursor).toEqual({
      afterEventId: "event-session-live",
      afterSequence: 2,
    });
    expect(harness.restoreCheckpoint).toHaveBeenCalledWith(
      streamIdentity("session-live"),
      expect.objectContaining({
        selectedTick: 12,
        syncIdentity: {
          backendScopeId: "backend-session-live",
          factorySessionId: "session-live",
          logicalSessionKeyId: "logical-session-live",
          streamGenerationId: "generation-session-live",
        },
      }),
    );
  });

  it("records stale-cursor recovery with the committed identity", async () => {
    const harness = createHarness(async ({ requestedSessionId }) =>
      resumeResolution(requestedSessionId, { staleCursorDetected: true }),
    );
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "session-stale",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    const correlationToken = correlationTokenForIdentityScope(
      streamIdentity("session-stale"),
    );
    expect(result.current.cursorFreeReplayCorrelationToken).toBe(
      correlationToken,
    );
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      expect.objectContaining({ outcome: "checkpoint_miss" }),
      expect.objectContaining({ outcome: "stale_cursor" }),
    ]);
  });
});

describe("dashboard checkpoint preflight core recovery", () => {
  it("surfaces non-recoverable recovery after clearing session state", async () => {
    const harness = createHarness(async ({ requestedSessionId }) => ({
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: true,
      kind: "recovery",
      reasonCode: "session_not_found",
      requestedSessionId,
    }));
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "missing-session",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(harness.effects.clearCheckpointsForSession).toHaveBeenCalledWith(
      window.indexedDB,
      "missing-session",
      { signal: expect.any(AbortSignal) },
    );
    expect(harness.effects.recoverSessionState).toHaveBeenCalledWith(
      harness.queryClient,
      "missing-session",
      expect.any(Function),
    );
    expect(result.current.preflightRecovery).toEqual({
      reasonCode: "session_not_found",
      requestedSessionId: "missing-session",
    });
    expect(harness.restoreCheckpoint).not.toHaveBeenCalled();
  });

  it("deletes an exact rejected checkpoint instead of clearing by session", async () => {
    const identity = streamIdentity("rejected-session");
    const persistedCheckpoint = {
      checkpoint: checkpoint(4),
      storageKey: "stored-checkpoint",
      streamIdentity: identity,
    };
    const harness = createHarness(async ({ requestedSessionId }) => ({
      checkpointToDelete: persistedCheckpoint,
      clearRequestedSessionCheckpoint: true,
      kind: "recovery",
      reasonCode: "stream_generation_changed",
      requestedSessionId,
    }));
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "rejected-session",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(harness.effects.deleteCheckpoint).toHaveBeenCalledWith(
      window.indexedDB,
      persistedCheckpoint,
      { signal: expect.any(AbortSignal) },
    );
    expect(harness.effects.clearCheckpointsForSession).not.toHaveBeenCalled();
  });

  it("marks resolution errors offline without marking preflight ready", async () => {
    const harness = createHarness(async ({ requestedSessionId }) => ({
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: true,
      error: new Error("network down"),
      kind: "error",
      requestedSessionId,
    }));
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "session-offline",
    });

    await waitFor(() => expect(result.current.checkpointHydrated).toBe(true));
    expect(result.current.preflightReady).toBe(false);
    expect(result.current.preflightError?.message).toBe("network down");
    expect(harness.setStreamState).toHaveBeenCalledWith({
      message: "network down",
      status: "offline",
    });
  });
});

describe("dashboard checkpoint preflight core identity commit", () => {
  it("records and commits a non-alias logical remap", async () => {
    const identity = streamIdentity("session-resolved");
    const harness = createHarness(async () => ({
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: true,
      identityRejectionDetail: "factory_session_mismatch",
      kind: "remap",
      requestedSessionId: "session-stale",
      resolvedSessionId: "session-resolved",
      streamIdentity: identity,
    }));
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "session-stale",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(harness.remapSelectedSessionID).toHaveBeenCalledWith(
      "session-resolved",
    );
    expect(
      readSessionPersistenceInvalidationRecords().map(({ outcome }) => outcome),
    ).toEqual(["checkpoint_hit", "identity_rejected", "logical_remap"]);
  });

  it("keeps default-alias resolution internal to the stream identity", async () => {
    const runtimeSessionID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";
    const harness = createHarness(async () => ({
      checkpoint: null,
      checkpointLookupOutcome: "checkpoint_miss",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: false,
      kind: "resume",
      requestedSessionId: "~default",
      resolvedSessionId: runtimeSessionID,
      staleCursorDetected: false,
      streamIdentity: streamIdentity(runtimeSessionID),
    }));
    const { result } = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "~default",
    });

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(harness.remapSelectedSessionID).not.toHaveBeenCalled();
    expect(result.current.resolvedSessionID).toBe(runtimeSessionID);
  });
});

describe("dashboard checkpoint preflight core cancellation", () => {
  it("ignores a superseded session resolution after the replacement commits", async () => {
    const sessionA = createDeferred<DashboardCheckpointPreflightResolution>();
    const sessionB = createDeferred<DashboardCheckpointPreflightResolution>();
    const harness = createHarness(({ requestedSessionId }) =>
      requestedSessionId === "session-a"
        ? sessionA.promise
        : sessionB.promise,
    );
    const hook = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "session-a",
    });
    await waitFor(() =>
      expect(harness.effects.resolvePreflight).toHaveBeenCalledTimes(1),
    );

    hook.rerender({ checkpointsDisabled: false, sessionID: "session-b" });
    await waitFor(() =>
      expect(harness.effects.resolvePreflight).toHaveBeenCalledTimes(2),
    );
    await act(async () => {
      sessionB.resolve(
        resumeResolution("session-b", { checkpoint: checkpoint(22) }),
      );
    });
    await waitFor(() =>
      expect(hook.result.current.resolvedSessionID).toBe("session-b"),
    );
    await act(async () => {
      sessionA.resolve({
        checkpointToDelete: null,
        clearRequestedSessionCheckpoint: true,
        kind: "remap",
        requestedSessionId: "session-a",
        resolvedSessionId: "session-a-remapped",
        streamIdentity: streamIdentity("session-a-remapped"),
      });
    });

    expect(hook.result.current.resolvedSessionID).toBe("session-b");
    expect(hook.result.current.persistedCheckpoint?.selectedTick).toBe(22);
    expect(harness.remapSelectedSessionID).not.toHaveBeenCalled();
    expect(harness.restoreCheckpoint).toHaveBeenCalledTimes(1);
  });

  it("does not recover session state after a clear is superseded", async () => {
    const clear = createDeferred<void>();
    const harness = createHarness(async ({ requestedSessionId }) => ({
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: true,
      kind: "recovery",
      reasonCode: "session_not_found",
      requestedSessionId,
    }));
    harness.effects.clearCheckpointsForSession.mockImplementation(
      async () => clear.promise,
    );
    const hook = renderPreflightCore(harness, {
      checkpointsDisabled: false,
      sessionID: "session-a",
    });
    await waitFor(() =>
      expect(
        harness.effects.clearCheckpointsForSession,
      ).toHaveBeenCalledTimes(1),
    );

    hook.rerender({ checkpointsDisabled: true, sessionID: "session-b" });
    await act(async () => clear.resolve());

    await waitFor(() => expect(hook.result.current.preflightReady).toBe(true));
    expect(hook.result.current.resolvedSessionID).toBe("session-b");
    expect(harness.effects.recoverSessionState).not.toHaveBeenCalled();
  });
});
