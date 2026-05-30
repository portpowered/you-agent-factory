import type { DashboardStreamState } from "../../../api/dashboard/types";

export interface DashboardWorldViewShellInput {
  eventCount: number;
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
  rawSessionID,
  selectedTick,
  streamState,
}: DashboardWorldViewShellInput): DashboardWorldViewShellState {
  const hasEvents = eventCount > 0;
  const hasNoStreamedSnapshot = selectedTick === 0 && !hasEvents;
  const isInitialLoading =
    rawSessionID != null &&
    hasNoStreamedSnapshot &&
    streamState.status !== "offline";
  const error =
    rawSessionID != null &&
    hasNoStreamedSnapshot &&
    streamState.status === "offline"
      ? new Error(streamState.message)
      : null;

  return {
    error,
    hasEvents,
    isInitialLoading,
  };
}
