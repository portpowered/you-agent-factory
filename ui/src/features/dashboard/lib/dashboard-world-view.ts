import type { DashboardStreamState } from "../../../api/dashboard/types";

export interface DashboardWorldViewShellInput {
  eventCount: number;
  hasRestoredCheckpoint: boolean;
  rawSessionID: string | null;
  selectedTick: number;
  streamState: DashboardStreamState;
}

export interface DashboardWorldViewShellState {
  error: Error | null;
  hasEvents: boolean;
  isInitialLoading: boolean;
}

export function deriveDashboardWorldViewShellState({
  eventCount,
  hasRestoredCheckpoint,
  rawSessionID,
  selectedTick,
  streamState,
}: DashboardWorldViewShellInput): DashboardWorldViewShellState {
  const hasEvents = eventCount > 0;
  const hasNoStreamedSnapshot =
    selectedTick === 0 && !hasEvents && !hasRestoredCheckpoint;
  const isInitialLoading =
    rawSessionID != null &&
    hasNoStreamedSnapshot &&
    streamState.status !== "offline" &&
    streamState.status !== "reconnecting" &&
    streamState.status !== "recovery_failed";
  const error =
    rawSessionID != null &&
    hasNoStreamedSnapshot &&
    (streamState.status === "offline" ||
      streamState.status === "recovery_failed")
      ? new Error(streamState.message)
      : null;

  return {
    error,
    hasEvents,
    isInitialLoading,
  };
}
