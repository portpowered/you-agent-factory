import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as timelinePublic from "../../../timeline/public/checkpoint-persistence";
import * as preflightResolver from "../../lib/preflight/resolve-dashboard-checkpoint-preflight";
import {
  correlationTokenForIdentityScope,
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../../lib/session-persistence/diagnostics";
import { useDashboardCheckpointPreflight } from "./use-dashboard-checkpoint-preflight";

const { remapSelectedSessionID } = vi.hoisted(() => ({
  remapSelectedSessionID: vi.fn(),
}));

vi.mock("../../session/dashboard-session-provider", () => ({
  useRemapDashboardSelectedSession: () => remapSelectedSessionID,
}));

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useDashboardCheckpointPreflight identity diagnostics", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    remapSelectedSessionID.mockReset();
    resetSessionPersistenceInvalidationRecords();
  });

  it("records one rejection and one committed non-alias logical remap with stable correlation", async () => {
    const queryClient = new QueryClient();
    const restoreCheckpoint = vi.fn();
    const streamIdentity = {
      backendScopeID: "backend-scope-a",
      factorySessionID: "session-resolved",
      logicalSessionKeyID: "logical-default",
      streamGenerationID: "generation-2",
    };
    vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
    ).mockResolvedValue({
      checkpointLookupOutcome: "checkpoint_hit",
      checkpointToDelete: null,
      clearRequestedSessionCheckpoint: true,
      identityRejectionDetail: "factory_session_mismatch",
      kind: "remap",
      requestedSessionId: "session-stale",
      resolvedSessionId: "session-resolved",
      streamIdentity,
    });
    vi.spyOn(
      timelinePublic,
      "clearTimelineCheckpointsForSession",
    ).mockResolvedValue(undefined);

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "session-stale::0",
          checkpointsDisabled: false,
          rawSessionID: "session-stale",
          refreshToken: 0,
          restoreCheckpoint,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    const correlationToken = correlationTokenForIdentityScope(streamIdentity);
    expect(remapSelectedSessionID).toHaveBeenCalledTimes(1);
    expect(remapSelectedSessionID).toHaveBeenCalledWith("session-resolved");
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      {
        correlationToken,
        outcome: "checkpoint_hit",
        recoveryAction: "reuse_checkpoint",
      },
      {
        correlationToken,
        detail: "factory_session_mismatch",
        outcome: "identity_rejected",
        recoveryAction: "discard_rejected_checkpoint",
      },
      {
        correlationToken,
        outcome: "logical_remap",
        recoveryAction: "switch_to_resolved_session",
      },
    ]);
  });

  it("preserves default-alias resolution without recording a logical remap", async () => {
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
      requestedSessionId: "~default",
      resolvedSessionId: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
      streamIdentity: {
        backendScopeID: "backend-a",
        factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
        logicalSessionKeyID: "logical-default",
        streamGenerationID: "generation-a",
      },
    });

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "~default::0",
          checkpointsDisabled: false,
          rawSessionID: "~default",
          refreshToken: 0,
          restoreCheckpoint,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(remapSelectedSessionID).not.toHaveBeenCalled();
    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      expect.objectContaining({ outcome: "checkpoint_miss" }),
    ]);
  });
});
