import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as preflightRunner from "../../lib/preflight/run-dashboard-checkpoint-preflight";
import { useDashboardCheckpointPreflight } from "./use-dashboard-checkpoint-preflight";

vi.mock("../../session/dashboard-session-provider", () => ({
  useRemapDashboardSelectedSession: () => vi.fn(),
}));

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useDashboardCheckpointPreflight", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
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
    vi.spyOn(preflightRunner, "runDashboardCheckpointPreflight").mockResolvedValue(
      {
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
      },
    );

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
