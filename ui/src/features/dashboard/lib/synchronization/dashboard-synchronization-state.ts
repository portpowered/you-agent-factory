export type DashboardPreflightOutcome =
  | { status: "pending" }
  | { status: "validated" }
  | { error: Error; status: "failed" };

export type DashboardCheckpointHydrationOutcome =
  | { status: "pending" }
  | { status: "reusable" }
  | { status: "absent" };

export type DashboardReplayOutcome =
  | { status: "pending" }
  | { status: "complete" };

export type DashboardStreamOpenOutcome =
  | { status: "pending" }
  | { status: "open" };

export type DashboardConnectivityOutcome =
  | { status: "connecting" }
  | { status: "live" }
  | { message: string; status: "offline" }
  | { message: string; status: "reconnecting" }
  | { message: string; status: "recovery_failed" };

export interface DashboardSynchronizationShellInput {
  checkpoint: DashboardCheckpointHydrationOutcome;
  connectivity: DashboardConnectivityOutcome;
  preflight: DashboardPreflightOutcome;
  replay: DashboardReplayOutcome;
  session: "none" | "selected";
  stream: DashboardStreamOpenOutcome;
}

export type DashboardSynchronizationShellStatus =
  | "idle"
  | "loading"
  | "current"
  | "known_empty"
  | "offline"
  | "reconnecting"
  | "failed"
  | "recovery_failed";

export interface DashboardSynchronizationShellState {
  error: Error | null;
  isInitialLoading: boolean;
  status: DashboardSynchronizationShellStatus;
}

export function deriveDashboardSynchronizationShellState({
  checkpoint,
  connectivity,
  preflight,
  replay,
  session,
  stream,
}: DashboardSynchronizationShellInput): DashboardSynchronizationShellState {
  if (session === "none") {
    return shellState("idle");
  }
  if (preflight.status === "failed") {
    return shellState("failed", preflight.error);
  }
  if (connectivity.status === "recovery_failed") {
    return shellState("recovery_failed", new Error(connectivity.message));
  }
  if (connectivity.status === "offline") {
    return shellState(
      "offline",
      checkpoint.status === "reusable" ? null : new Error(connectivity.message),
    );
  }
  if (connectivity.status === "reconnecting") {
    return shellState("reconnecting");
  }

  const synchronizationComplete =
    preflight.status === "validated" &&
    checkpoint.status !== "pending" &&
    replay.status === "complete" &&
    stream.status === "open" &&
    connectivity.status === "live";
  if (!synchronizationComplete) {
    return shellState("loading");
  }

  return shellState(checkpoint.status === "reusable" ? "current" : "known_empty");
}

function shellState(
  status: DashboardSynchronizationShellStatus,
  error: Error | null = null,
): DashboardSynchronizationShellState {
  return {
    error,
    isInitialLoading: status === "loading",
    status,
  };
}
