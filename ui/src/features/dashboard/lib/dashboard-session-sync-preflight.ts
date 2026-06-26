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
  reconnectCursor?: FactoryEventReconnectCursor;
  restoreCheckpoint: FactoryTimelineCheckpoint | null;
  syncIdentity: FactoryTimelineSyncIdentity | null;
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
    return {
      clearCheckpoint: false,
      clearQueryCache:
        preflight.reasonCode !== "ok" || syncIdentity == null,
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
      reconnectCursor,
      restoreCheckpoint: checkpoint,
      syncIdentity,
    };
  }

  if (preflight.reasonCode === "cursor_stale") {
    return {
      clearCheckpoint: true,
      clearQueryCache: false,
      reconnectCursor: undefined,
      restoreCheckpoint: null,
      syncIdentity,
    };
  }

  const missingIdentity =
    syncIdentity == null || checkpoint.syncIdentity == null;

  return {
    clearCheckpoint: true,
    clearQueryCache:
      preflight.reasonCode !== "ok" || missingIdentity || !checkpointIdentityMatches,
    reconnectCursor: undefined,
    restoreCheckpoint: null,
    syncIdentity,
  };
}
