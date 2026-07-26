import type { FactoryEventReconnectCursor } from "../../../../api/events";
import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import type { TimelineCheckpointStreamIdentity } from "../../../timeline/public/checkpoint-persistence";
import type { FactoryTimelineSyncIdentity } from "../../../timeline/public/store";

export interface DashboardSessionRecoveryState {
  reasonCode: string;
  requestedSessionId: string;
}

export type DashboardSyncPreflightStreamIdentity =
  TimelineCheckpointStreamIdentity;

export type DashboardSyncPreflightResolution =
  | {
      checkpointReusable: boolean;
      kind: "resume";
      reconnectCursor?: FactoryEventReconnectCursor;
      requestedSessionId: string;
      resolvedSessionId: string;
      streamIdentity: DashboardSyncPreflightStreamIdentity;
    }
  | {
      kind: "non-recoverable";
      recovery: DashboardSessionRecoveryState;
    };

const RESUMABLE_REASON_CODES = new Set<string>([
  FactorySessionSyncPreflightReasonCode.ok,
  FactorySessionSyncPreflightReasonCode.cursor_stale,
  FactorySessionSyncPreflightReasonCode.logical_session_remap,
]);

const NON_RECOVERABLE_REASON_CODES = new Set<string>([
  FactorySessionSyncPreflightReasonCode.session_not_found,
  FactorySessionSyncPreflightReasonCode.invalid_target_reference,
]);

export function isNonRecoverableSyncPreflightReasonCode(
  reasonCode: string,
): boolean {
  return NON_RECOVERABLE_REASON_CODES.has(reasonCode);
}

export function syncPreflightIdentityHintsFromCheckpoint(
  checkpointSyncIdentity?: FactoryTimelineSyncIdentity,
  streamIdentity?: TimelineCheckpointStreamIdentity | null,
): {
  backendScopeId?: string;
  logicalSessionKeyId?: string;
} {
  const backendScopeId =
    checkpointSyncIdentity?.backendScopeId?.trim() ||
    streamIdentity?.backendScopeID?.trim();
  const logicalSessionKeyId =
    checkpointSyncIdentity?.logicalSessionKeyId?.trim() ||
    streamIdentity?.logicalSessionKeyID?.trim();

  return {
    ...(backendScopeId ? { backendScopeId } : {}),
    ...(logicalSessionKeyId ? { logicalSessionKeyId } : {}),
  };
}

export function resolveDashboardSyncPreflight(
  response: FactorySessionSyncPreflightResponse,
): DashboardSyncPreflightResolution {
  if (isNonRecoverableSyncPreflightReasonCode(response.reasonCode)) {
    return {
      kind: "non-recoverable",
      recovery: {
        reasonCode: response.reasonCode,
        requestedSessionId: response.requestedSessionId,
      },
    };
  }

  if (!RESUMABLE_REASON_CODES.has(response.reasonCode)) {
    return {
      kind: "non-recoverable",
      recovery: {
        reasonCode: response.reasonCode,
        requestedSessionId: response.requestedSessionId,
      },
    };
  }

  const streamIdentity = streamIdentityFromSyncPreflightResponse(response);
  if (!streamIdentity) {
    return {
      kind: "non-recoverable",
      recovery: {
        reasonCode:
          FactorySessionSyncPreflightReasonCode.invalid_target_reference,
        requestedSessionId: response.requestedSessionId,
      },
    };
  }

  return {
    checkpointReusable: response.checkpointReusable,
    kind: "resume",
    reconnectCursor: reconnectCursorFromSyncPreflightResponse(response),
    requestedSessionId: response.requestedSessionId,
    resolvedSessionId: streamIdentity.factorySessionID,
    streamIdentity,
  };
}

function streamIdentityFromSyncPreflightResponse(
  response: FactorySessionSyncPreflightResponse,
): DashboardSyncPreflightStreamIdentity | null {
  const backendScopeID = response.backendScopeId?.trim();
  const factorySessionID = response.factorySessionId?.trim();
  const logicalSessionKeyId = response.logicalSessionKeyId?.trim();
  const streamGenerationID = response.streamGenerationId?.trim();

  if (
    !backendScopeID ||
    !factorySessionID ||
    !logicalSessionKeyId ||
    !streamGenerationID
  ) {
    return null;
  }

  return {
    backendScopeID,
    factorySessionID,
    logicalSessionKeyID: logicalSessionKeyId,
    streamGenerationID,
  };
}

function reconnectCursorFromSyncPreflightResponse(
  response: FactorySessionSyncPreflightResponse,
): FactoryEventReconnectCursor | undefined {
  if (
    response.reasonCode !== FactorySessionSyncPreflightReasonCode.ok ||
    !response.checkpointReusable ||
    !response.reconnectCursor.provided ||
    !response.reconnectCursor.validForStreamGeneration
  ) {
    return undefined;
  }

  const afterEventId = response.reconnectCursor.afterEventId?.trim();
  const afterSequence = response.reconnectCursor.afterSequence;
  if (!afterEventId && afterSequence == null) {
    return undefined;
  }

  return {
    afterEventId,
    afterSequence,
  };
}

export function shouldClearCheckpointAfterPreflight(
  response: FactorySessionSyncPreflightResponse,
): boolean {
  return (
    response.reasonCode ===
      FactorySessionSyncPreflightReasonCode.cursor_stale ||
    response.reasonCode ===
      FactorySessionSyncPreflightReasonCode.logical_session_remap ||
    !response.checkpointReusable
  );
}

export function shouldRemapDashboardSession(
  response: FactorySessionSyncPreflightResponse,
  requestedSessionId: string,
): boolean {
  const resolvedSessionId = response.factorySessionId?.trim();
  return (
    response.reasonCode ===
      FactorySessionSyncPreflightReasonCode.logical_session_remap ||
    (resolvedSessionId != null && resolvedSessionId !== requestedSessionId)
  );
}

export function checkpointSyncIdentityFromPreflight(
  streamIdentity: DashboardSyncPreflightStreamIdentity,
): FactoryTimelineSyncIdentity {
  return {
    backendScopeId: streamIdentity.backendScopeID,
    factorySessionId: streamIdentity.factorySessionID,
    logicalSessionKeyId: streamIdentity.logicalSessionKeyID,
    streamGenerationId: streamIdentity.streamGenerationID,
  };
}
