import type {
  DashboardSnapshot,
  DashboardStreamState,
} from "../../../api/dashboard/types";
import type { FactoryTimelineMode } from "../../timeline/state/timeline/storeState";

export type DashboardCardContentState =
  | "error"
  | "known-empty"
  | "loading"
  | "populated";

export type DashboardCardFreshnessState =
  | "failed"
  | "fresh"
  | "reconnecting"
  | "recovering"
  | "stale";

export type DashboardCardTemporalState = "historical" | "live";

export interface DashboardCardAsyncState {
  content: DashboardCardContentState;
  freshness: DashboardCardFreshnessState;
  temporal: DashboardCardTemporalState;
}

export interface DashboardCardStateContext {
  hasAuthoritativeSnapshot: boolean;
  recoveryPending: boolean;
  streamStatus: DashboardStreamState["status"];
  timelineMode: FactoryTimelineMode;
}

export interface DashboardCardStateSnapshot {
  value: unknown;
  widgetType: string;
}

export interface DashboardCardContentInput {
  authoritative: boolean;
  error?: boolean;
  hasRecords: boolean;
  loading?: boolean;
}

export interface DashboardCardAsyncStateInput
  extends DashboardCardStateContext {
  content: DashboardCardContentState;
  hasRetainedData?: boolean;
}

/**
 * A card can report known-empty only after its own authoritative input has
 * completed. This keeps an empty replay or chart from masquerading as a
 * loaded zero while session or generation hydration is still pending.
 */
export function resolveDashboardCardContentState({
  authoritative,
  error = false,
  hasRecords,
  loading = false,
}: DashboardCardContentInput): DashboardCardContentState {
  if (error) {
    return "error";
  }
  if (!authoritative || loading) {
    return "loading";
  }
  return hasRecords ? "populated" : "known-empty";
}

export function resolveDashboardCardAsyncState({
  content,
  hasRetainedData = content === "populated",
  recoveryPending,
  streamStatus,
  timelineMode,
}: DashboardCardAsyncStateInput): DashboardCardAsyncState {
  const freshness = resolveDashboardCardFreshness({
    content,
    hasRetainedData,
    recoveryPending,
    streamStatus,
  });

  return {
    content: normalizeDashboardCardContentForFreshness({
      content,
      freshness,
    }),
    freshness,
    temporal: timelineMode === "fixed" ? "historical" : "live",
  };
}

function normalizeDashboardCardContentForFreshness({
  content,
  freshness,
}: {
  content: DashboardCardContentState;
  freshness: DashboardCardFreshnessState;
}): DashboardCardContentState {
  if (content !== "known-empty") {
    return content;
  }
  if (freshness === "failed") {
    return "error";
  }
  if (freshness === "reconnecting" || freshness === "recovering") {
    return "loading";
  }
  return content;
}

function resolveDashboardCardFreshness({
  content,
  hasRetainedData,
  recoveryPending,
  streamStatus,
}: {
  content: DashboardCardContentState;
  hasRetainedData: boolean;
  recoveryPending: boolean;
  streamStatus: DashboardStreamState["status"];
}): DashboardCardFreshnessState {
  if (streamStatus === "recovery_failed" || content === "error") {
    return "failed";
  }
  if (streamStatus === "reconnecting") {
    return "reconnecting";
  }
  if (streamStatus === "offline") {
    return hasRetainedData ? "stale" : "failed";
  }
  if (streamStatus === "connecting") {
    if (recoveryPending) {
      return "recovering";
    }
    return "reconnecting";
  }
  return "fresh";
}

export function dashboardSnapshotHasRecords(
  snapshot: DashboardSnapshot,
): boolean {
  return (
    snapshot.runtime.session.has_data ||
    snapshot.runtime.in_flight_dispatch_count > 0 ||
    (snapshot.runtime.active_dispatch_ids?.length ?? 0) > 0
  );
}
