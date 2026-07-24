import { describe, expect, it, vi } from "vitest";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import { resolveDashboardCheckpointPreflight } from "./resolve-dashboard-checkpoint-preflight";

const identity = {
  backendScopeID: "backend-a",
  factorySessionID: "session-a",
  logicalSessionKeyID: "logical-a",
  streamGenerationID: "generation-a",
};

function response(overrides: Record<string, unknown> = {}) {
  return {
    backendScopeId: identity.backendScopeID,
    checkpointReusable: true,
    factorySessionId: identity.factorySessionID,
    logicalSessionKeyId: identity.logicalSessionKeyID,
    reasonCode: FactorySessionSyncPreflightReasonCode.ok,
    reconnectCursor: {
      afterEventId: "event-7",
      afterSequence: 7,
      provided: true,
      validForStreamGeneration: true,
    },
    requestedSessionId: identity.factorySessionID,
    streamGenerationId: identity.streamGenerationID,
    ...overrides,
  } as never;
}

function dependencies(preflightResponse = response()) {
  return {
    getSyncPreflight: vi.fn().mockResolvedValue(preflightResponse),
    peekCheckpoint: vi.fn().mockResolvedValue({
      checkpoint: {
        afterEventId: "event-7",
        afterSequence: 7,
        replayState: {},
        selectedTick: 7,
      },
      storageKey: "checkpoint-session-a",
      streamIdentity: identity,
    }),
    readCheckpoint: vi.fn().mockResolvedValue({
      afterEventId: "event-7",
      afterSequence: 7,
      replayState: {},
      selectedTick: 7,
    }),
  };
}

describe("resolveDashboardCheckpointPreflight", () => {
  it("returns a reusable resume outcome without invoking mutation collaborators", async () => {
    const deps = dependencies();
    const clearCheckpoint = vi.fn();
    const clearQueries = vi.fn();
    const remapSession = vi.fn();
    const restoreCheckpoint = vi.fn();
    const setStreamState = vi.fn();

    const result = await resolveDashboardCheckpointPreflight({
      dependencies: deps,
      indexedDB: undefined,
      requestedSessionId: "session-a",
    });

    expect(result).toMatchObject({
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: false,
      kind: "resume",
      reconnectCursor: { afterEventId: "event-7", afterSequence: 7 },
      requestedSessionId: "session-a",
      resolvedSessionId: "session-a",
      streamIdentity: identity,
    });
    expect(deps.readCheckpoint).toHaveBeenCalledWith(undefined, identity, {
      signal: undefined,
    });
    for (const collaborator of [
      clearCheckpoint,
      clearQueries,
      remapSession,
      restoreCheckpoint,
      setStreamState,
    ]) {
      expect(collaborator).not.toHaveBeenCalled();
    }
  });

  it("returns explicit remap and invalidation decisions", async () => {
    const deps = dependencies(
      response({
        checkpointReusable: false,
        factorySessionId: "session-b",
        reasonCode: FactorySessionSyncPreflightReasonCode.logical_session_remap,
        requestedSessionId: "session-a",
      }),
    );

    await expect(
      resolveDashboardCheckpointPreflight({
        dependencies: deps,
        indexedDB: undefined,
        requestedSessionId: "session-a",
      }),
    ).resolves.toMatchObject({
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: expect.objectContaining({
        storageKey: "checkpoint-session-a",
      }),
      clearRequestedSessionCheckpoint: true,
      identityRejectionDetail: "factory_session_mismatch",
      kind: "remap",
      requestedSessionId: "session-a",
      resolvedSessionId: "session-b",
    });
    expect(deps.readCheckpoint).not.toHaveBeenCalled();
  });
});

describe("resolveDashboardCheckpointPreflight identity rejection", () => {
  it.each([
    [
      "backend scope",
      { backendScopeID: "backend-stale" },
      "backend_scope_mismatch",
    ],
    [
      "factory session",
      { factorySessionID: "session-stale" },
      "factory_session_mismatch",
    ],
    [
      "logical session",
      { logicalSessionKeyID: "logical-stale" },
      "logical_session_mismatch",
    ],
    [
      "stream generation",
      { streamGenerationID: "generation-stale" },
      "stream_generation_mismatch",
    ],
  ] as const)(
    "returns a bounded identity rejection detail for a %s mismatch",
    async (_label, mismatch, expectedDetail) => {
      const deps = dependencies();
      deps.peekCheckpoint.mockResolvedValue({
        checkpoint: {
          afterEventId: "event-7",
          afterSequence: 7,
          replayState: {},
          selectedTick: 7,
        },
        storageKey: "sensitive-storage-key",
        streamIdentity: { ...identity, ...mismatch },
      });

      const result = await resolveDashboardCheckpointPreflight({
        dependencies: deps,
        indexedDB: undefined,
        requestedSessionId: "session-a",
      });

      expect(result).toMatchObject({
        clearRequestedSessionCheckpoint: true,
        identityRejectionDetail: expectedDetail,
      });
      expect(result).not.toHaveProperty("identityRejectionScope");
      expect(deps.readCheckpoint).not.toHaveBeenCalled();
    },
  );
});

describe("resolveDashboardCheckpointPreflight failures", () => {
  it("returns recovery for unresolved sessions", async () => {
    const deps = dependencies(
      response({
        checkpointReusable: false,
        reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
        reconnectCursor: { provided: false, validForStreamGeneration: false },
      }),
    );

    await expect(
      resolveDashboardCheckpointPreflight({
        dependencies: deps,
        indexedDB: undefined,
        requestedSessionId: "session-a",
      }),
    ).resolves.toEqual({
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: expect.objectContaining({
        storageKey: "checkpoint-session-a",
      }),
      clearRequestedSessionCheckpoint: true,
      kind: "recovery",
      reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
      requestedSessionId: "session-a",
    });
  });

  it("returns typed errors but propagates cancellation", async () => {
    const failed = dependencies();
    failed.getSyncPreflight.mockRejectedValue(new Error("offline"));
    await expect(
      resolveDashboardCheckpointPreflight({
        dependencies: failed,
        indexedDB: undefined,
        requestedSessionId: "session-a",
      }),
    ).resolves.toMatchObject({
      checkpointLookupOutcome: "checkpoint_hit",
      kind: "error",
      requestedSessionId: "session-a",
    });

    const controller = new AbortController();
    const cancelled = dependencies();
    cancelled.getSyncPreflight.mockImplementation(async () => {
      controller.abort();
      throw new DOMException("Aborted", "AbortError");
    });
    await expect(
      resolveDashboardCheckpointPreflight({
        dependencies: cancelled,
        indexedDB: undefined,
        requestedSessionId: "session-a",
        signal: controller.signal,
      }),
    ).rejects.toMatchObject({ name: "AbortError" });
    expect(cancelled.peekCheckpoint).toHaveBeenCalledWith(
      undefined,
      "session-a",
      { signal: controller.signal },
    );
  });
});

describe("resolveDashboardCheckpointPreflight lookup diagnostics", () => {
  it("reports a miss only after a completed empty lookup", async () => {
    const deps = dependencies();
    deps.peekCheckpoint.mockResolvedValue(null);

    await expect(
      resolveDashboardCheckpointPreflight({
        dependencies: deps,
        indexedDB: undefined,
        requestedSessionId: "session-a",
      }),
    ).resolves.toMatchObject({ checkpointLookupOutcome: "checkpoint_miss" });

    const failedLookup = dependencies();
    failedLookup.peekCheckpoint.mockRejectedValue(new Error("lookup failed"));
    await expect(
      resolveDashboardCheckpointPreflight({
        dependencies: failedLookup,
        indexedDB: undefined,
        requestedSessionId: "session-a",
      }),
    ).resolves.not.toHaveProperty("checkpointLookupOutcome");
  });
});
