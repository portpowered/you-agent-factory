import type {
  FactoryEventReconnectCursor,
} from "../../../api/events";
import type {
  FactorySessionSyncPreflightResponse,
} from "../../../api/factory-sessions";
import type {
  FactoryTimelineCheckpoint,
  FactoryTimelineSyncIdentity,
} from "../../timeline/public";
import { reconnectCursorFromCheckpoint } from "../../timeline/public";

export interface DashboardSessionRestorePlan {
  clearCheckpoint: boolean;
  clearQueryCache: boolean;
  preflightStatus: DashboardSessionPreflightStatus;
  recoveryState: DashboardSessionRecoveryState | null;
  reconnectCursor?: FactoryEventReconnectCursor;
  restoreCheckpoint: FactoryTimelineCheckpoint | null;
  syncIdentity: FactoryTimelineSyncIdentity | null;
}

export type DashboardSessionPreflightStatus =
  | "idle"
  | "loading"
  | "success"
  | "silent-recovery"
  | "non-recoverable";

export interface DashboardSessionRecoveryState {
  reasonCode: string;
  requestedSessionId: string;
}

export function buildDashboardSessionPreflightFailureRecoveryState(
  requestedSessionId: string,
): DashboardSessionRecoveryState {
  return {
    reasonCode: "preflight_request_failed",
    requestedSessionId,
  };
}

export function syncIdentityFromPreflight(
  preflight: FactorySessionSyncPreflightResponse,
): FactoryTimelineSyncIdentity | null {
  if (
    typeof preflight.backendScopeId !== "string" ||
    typeof preflight.factorySessionId !== "string" ||
    typeof preflight.logicalSessionKeyId !== "string" ||
    typeof preflight.streamGenerationId !== "string"
  ) {
    return null;
  }

  return {
    backendScopeId: preflight.backendScopeId,
    factorySessionId: preflight.factorySessionId,
    logicalSessionKeyId: preflight.logicalSessionKeyId,
    streamGenerationId: preflight.streamGenerationId,
  };
}

function syncIdentityMatches(
  left: FactoryTimelineSyncIdentity | undefined,
  right: FactoryTimelineSyncIdentity | null,
): boolean {
  return (
    left?.backendScopeId === right?.backendScopeId &&
    left?.factorySessionId === right?.factorySessionId &&
    left?.logicalSessionKeyId === right?.logicalSessionKeyId &&
    left?.streamGenerationId === right?.streamGenerationId
  );
}

function isNonRecoverableReasonCode(reasonCode: string): boolean {
  return (
    reasonCode === "session_not_found" ||
    (reasonCode !== "ok" &&
      reasonCode !== "cursor_stale" &&
      reasonCode !== "logical_session_remap")
  );
}

export function buildDashboardSessionRestorePlan(
  checkpoint: FactoryTimelineCheckpoint | null,
  preflight: FactorySessionSyncPreflightResponse,
): DashboardSessionRestorePlan {
  const syncIdentity = syncIdentityFromPreflight(preflight);
  const reconnectCursor = reconnectCursorFromCheckpoint(checkpoint);
  const checkpointHasCursor = reconnectCursor != null;
  const checkpointIdentityMatches = syncIdentityMatches(
    checkpoint?.syncIdentity,
    syncIdentity,
  );

  if (checkpoint == null) {
    const nonRecoverable = isNonRecoverableReasonCode(preflight.reasonCode);
    return {
      clearCheckpoint: false,
      clearQueryCache:
        preflight.reasonCode !== "ok" || syncIdentity == null,
      preflightStatus: nonRecoverable
        ? "non-recoverable"
        : preflight.reasonCode === "ok"
          ? "success"
          : "silent-recovery",
      recoveryState: nonRecoverable
        ? {
            reasonCode: preflight.reasonCode,
            requestedSessionId: preflight.requestedSessionId,
          }
        : null,
      reconnectCursor: undefined,
      restoreCheckpoint: null,
      syncIdentity,
    };
  }

  const canRestoreCheckpoint =
    preflight.reasonCode === "ok" &&
    preflight.checkpointReusable &&
    preflight.reconnectCursor.provided &&
    preflight.reconnectCursor.validForStreamGeneration &&
    checkpointHasCursor &&
    checkpointIdentityMatches;

  if (canRestoreCheckpoint) {
    return {
      clearCheckpoint: false,
      clearQueryCache: false,
      preflightStatus: "success",
      recoveryState: null,
      reconnectCursor,
      restoreCheckpoint: checkpoint,
      syncIdentity,
    };
  }

  if (preflight.reasonCode === "cursor_stale") {
    return {
      clearCheckpoint: true,
      clearQueryCache: false,
      preflightStatus: "silent-recovery",
      recoveryState: null,
      reconnectCursor: undefined,
      restoreCheckpoint: null,
      syncIdentity,
    };
  }

  const missingIdentity =
    syncIdentity == null || checkpoint.syncIdentity == null;
  const nonRecoverable = isNonRecoverableReasonCode(preflight.reasonCode);

  return {
    clearCheckpoint: true,
    clearQueryCache:
      preflight.reasonCode !== "ok" || missingIdentity || !checkpointIdentityMatches,
    preflightStatus: nonRecoverable
      ? "non-recoverable"
      : "silent-recovery",
    recoveryState: nonRecoverable
      ? {
          reasonCode: preflight.reasonCode,
          requestedSessionId: preflight.requestedSessionId,
        }
      : null,
    reconnectCursor: undefined,
    restoreCheckpoint: null,
    syncIdentity,
  };
}
