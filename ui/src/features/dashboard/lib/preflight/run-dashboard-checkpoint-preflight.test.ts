import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import * as factorySessionsAPI from "../../../../api/factory-sessions";
import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import * as timelinePublic from "../../../timeline/public";
import { runDashboardCheckpointPreflight } from "./run-dashboard-checkpoint-preflight";

const RESOLVED_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

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

  return {
    clearCheckpointsSpy,
    getSyncPreflightSpy,
    peekCheckpointSpy,
    queryClient,
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
  let queryClient: ReturnType<typeof installPreflightMocks>["queryClient"];

  beforeEach(() => {
    ({ clearCheckpointsSpy, getSyncPreflightSpy, peekCheckpointSpy, queryClient } =
      installPreflightMocks());
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
      onRemapSessionID: vi.fn(),
      onStreamOffline: vi.fn(),
      queryClient: queryClient as never,
      rawSessionID: RESOLVED_SESSION_UUID,
      restoreCheckpoint: vi.fn(),
    });

    expect(clearCheckpointsSpy).toHaveBeenCalled();
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
});
