import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as factorySessionsAPI from "../../../../api/factory-sessions";
import * as timelinePublic from "../../../timeline/public/checkpoint-persistence";
import * as preflightResolver from "../../lib/preflight/resolve-dashboard-checkpoint-preflight";
import {
  createSessionPersistenceCorrelationToken,
  readSessionPersistenceDiagnosticRecords,
  resetSessionPersistenceDiagnosticRecords,
} from "../../lib/session-persistence/diagnostics";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";
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

beforeEach(() => {
  vi.restoreAllMocks();
  remapSelectedSessionID.mockReset();
  resetSessionPersistenceDiagnosticRecords();
  useDashboardStreamStore.setState({
    setStreamState: useDashboardStreamStore.getState().setStreamState,
    resetStreamState: useDashboardStreamStore.getState().resetStreamState,
  });
});

describe("useDashboardCheckpointPreflight recovery", () => {
  it("surfaces non-recoverable preflight recovery without stream identity", async () => {
    const queryClient = new QueryClient();
    vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
    ).mockResolvedValue({
      clearRequestedSessionCheckpoint: true,
      checkpointToDelete: null,
      kind: "recovery",
      reasonCode: "session_not_found",
      requestedSessionId: "missing-session",
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

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(result.current.preflightRecovery).toEqual({
      reasonCode: "session_not_found",
      requestedSessionId: "missing-session",
    });
    expect(result.current.resolvedSessionID).toBeNull();
    expect(result.current.streamIdentity).toBeNull();
  });

  it("records a rejected checkpoint lookup without claiming restoration", async () => {
    const queryClient = new QueryClient();
    const restoreCheckpoint = vi.fn();
    vi.spyOn(
      timelinePublic,
      "clearTimelineCheckpointsForSession",
    ).mockResolvedValue(undefined);
    vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
    ).mockResolvedValue({
      checkpointLookupOutcome: "checkpoint_hit",
      clearRequestedSessionCheckpoint: true,
      checkpointToDelete: null,
      kind: "recovery",
      reasonCode: "session_not_found",
      requestedSessionId: "rejected-session",
    });

    const { result } = renderHook(
      () =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: "rejected-session::0",
          checkpointsDisabled: false,
          rawSessionID: "rejected-session",
          refreshToken: 0,
          restoreCheckpoint,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => expect(result.current.preflightReady).toBe(true));
    expect(restoreCheckpoint).not.toHaveBeenCalled();
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([
      {
        correlationToken:
          createSessionPersistenceCorrelationToken("rejected-session"),
        outcome: "checkpoint_hit",
        recoveryAction: "reuse_checkpoint",
      },
    ]);
  });
});

describe("useDashboardCheckpointPreflight errors", () => {
  it("hydrates checkpoint state on preflight error without marking ready", async () => {
    const queryClient = new QueryClient();
    const restoreCheckpoint = vi.fn();
    vi.spyOn(
      preflightResolver,
      "resolveDashboardCheckpointPreflight",
    ).mockResolvedValue({
      clearRequestedSessionCheckpoint: true,
      checkpointToDelete: null,
      error: new Error("validation failed"),
      kind: "error",
      requestedSessionId: "session-live-001",
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

    await waitFor(() => expect(result.current.checkpointHydrated).toBe(true));
    expect(result.current.preflightReady).toBe(false);
    expect(result.current.preflightError?.message).toBe("validation failed");
  });

  it("marks the stream offline when checkpoint preflight rejects", async () => {
    const queryClient = new QueryClient();
    const restoreCheckpoint = vi.fn();
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
          restoreCheckpoint,
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
