import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as factorySessionsAPI from "../../../../api/factory-sessions";
import * as timelinePublic from "../../../timeline/public";
import * as preflightRunner from "../../lib/preflight/run-dashboard-checkpoint-preflight";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
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

function resetCheckpointPreflightTestState(): void {
  vi.restoreAllMocks();
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

describe("useDashboardCheckpointPreflight recovery and errors", () => {
  beforeEach(() => {
    resetCheckpointPreflightTestState();
  });

  it("surfaces non-recoverable preflight recovery without stream identity", async () => {
    const queryClient = new QueryClient();
    vi.spyOn(preflightRunner, "runDashboardCheckpointPreflight").mockResolvedValue(
      {
        initialReconnectCursor: undefined,
        persistedCheckpoint: null,
        preflightError: null,
        preflightRecovery: {
          reasonCode: "session_not_found",
          requestedSessionId: "missing-session",
        },
        resolvedSessionID: null,
        streamIdentity: null,
      },
    );

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
    vi.spyOn(preflightRunner, "runDashboardCheckpointPreflight").mockResolvedValue(
      {
        initialReconnectCursor: undefined,
        persistedCheckpoint: null,
        preflightError: new Error("validation failed"),
        preflightRecovery: null,
        resolvedSessionID: null,
        streamIdentity: null,
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
      expect(result.current.checkpointHydrated).toBe(true);
    });
    expect(result.current.preflightReady).toBe(false);
    expect(result.current.preflightError?.message).toBe("validation failed");
  });

  it("marks the stream offline when checkpoint preflight rejects", async () => {
    const queryClient = new QueryClient();
    const setStreamState = vi.fn();
    useDashboardStreamStore.setState({ setStreamState });
    vi.spyOn(timelinePublic, "peekPersistedTimelineCheckpoint").mockResolvedValue(
      null,
    );
    vi.spyOn(
      timelinePublic,
      "clearTimelineCheckpointsForSession",
    ).mockResolvedValue(undefined);
    vi.spyOn(factorySessionsAPI, "getFactorySessionSyncPreflight").mockRejectedValue(
      new Error("network down"),
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
