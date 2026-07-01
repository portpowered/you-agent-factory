import type { FactoryEventReconnectCursor } from "../../../api/events";
import {
  type FactorySessionLogicalResolveHint,
  type FactorySessionSyncPreflightResponse,
  getFactorySessionSyncPreflight,
} from "../../../api/factory-sessions/sync-preflight";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
} from "../../timeline/lib/stream-derived-cache-identity";
import {
  clearStoredTimelineCheckpointsForFactorySessionID,
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  findStoredCheckpointEnvelopeByFactorySessionID,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timeline/public";

export interface DashboardSessionRecoveryState {
  reasonCode: string;
  requestedSessionId: string;
}

export interface DashboardSessionSyncPreflightBootstrap {
  checkpoint: FactoryTimelineCheckpoint | null;
  reconnectCursor: FactoryEventReconnectCursor | undefined;
  remappedFactorySessionId: string | null;
  streamIdentity: StreamDerivedCacheIdentity;
}

type DashboardSessionSyncPreflightOutcome =
  | { kind: "ready"; result: DashboardSessionSyncPreflightBootstrap }
  | { kind: "recovery"; recovery: DashboardSessionRecoveryState }
  | { kind: "error"; error: Error };

interface IndexedDBLike {
  open: IDBFactory["open"];
}

const NON_RECOVERABLE_REASON_CODES = new Set([
  "session_not_found",
  "logical_session_unresolved",
]);

export function isNonRecoverableSyncPreflightReason(
  reasonCode: string,
): boolean {
  return NON_RECOVERABLE_REASON_CODES.has(reasonCode);
}

export function streamIdentityFromSyncPreflightResponse(
  response: FactorySessionSyncPreflightResponse,
): StreamDerivedCacheIdentity | null {
  return normalizeStreamDerivedCacheIdentity({
    backendScopeID: response.backendScopeId,
    factorySessionID: response.factorySessionId,
    logicalSessionKeyID: response.logicalSessionKeyId,
    streamGenerationID: response.streamGenerationId,
  });
}

function logicalResolveFromStreamIdentity(
  streamIdentity: StreamDerivedCacheIdentity,
): FactorySessionLogicalResolveHint {
  return {
    backendScopeID: streamIdentity.backendScopeID,
    logicalSessionKeyID: streamIdentity.logicalSessionKeyID,
  };
}

function recoveryFromResponse(
  response: FactorySessionSyncPreflightResponse,
): DashboardSessionRecoveryState {
  return {
    reasonCode: response.reasonCode,
    requestedSessionId: response.requestedSessionId,
  };
}

function syncPreflightError(
  error: unknown,
  fallbackMessage: string,
): DashboardSessionSyncPreflightOutcome {
  const message =
    error instanceof Error && error.message.trim() !== ""
      ? error.message
      : fallbackMessage;
  return { kind: "error", error: new Error(message) };
}

async function clearCheckpointForStreamIdentity(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<void> {
  if (!indexedDB || streamIdentity == null) {
    return;
  }
  await clearTimelineCheckpoint(indexedDB, streamIdentity);
}

async function attemptStoredCheckpointLogicalRemap({
  indexedDB,
  response,
  sessionID,
}: {
  indexedDB: IndexedDBLike | undefined;
  response: FactorySessionSyncPreflightResponse;
  sessionID: string;
}): Promise<
  | { kind: "remapped"; response: FactorySessionSyncPreflightResponse }
  | { kind: "error"; outcome: DashboardSessionSyncPreflightOutcome }
> {
  const storedEnvelope = await findStoredCheckpointEnvelopeByFactorySessionID(
    indexedDB,
    sessionID,
  );
  if (!storedEnvelope || response.reasonCode !== "session_not_found") {
    return { kind: "remapped", response };
  }

  const candidateCursor = reconnectCursorFromCheckpoint(
    storedEnvelope.checkpoint,
  );
  try {
    return {
      kind: "remapped",
      response: await getFactorySessionSyncPreflight(
        sessionID,
        candidateCursor,
        {
          logicalResolve: {
            backendScopeID: storedEnvelope.streamIdentity.backendScopeID,
            logicalSessionKeyID:
              storedEnvelope.streamIdentity.logicalSessionKeyID,
          },
        },
      ),
    };
  } catch (error: unknown) {
    return {
      kind: "error",
      outcome: syncPreflightError(
        error,
        "The dashboard could not load the selected session.",
      ),
    };
  }
}

function readyAfterLogicalRemap(
  response: FactorySessionSyncPreflightResponse,
  streamIdentity: StreamDerivedCacheIdentity,
): { kind: "ready"; result: DashboardSessionSyncPreflightBootstrap } {
  return {
    kind: "ready",
    result: {
      checkpoint: null,
      reconnectCursor: undefined,
      remappedFactorySessionId: response.factorySessionId ?? null,
      streamIdentity,
    },
  };
}

async function validateSavedReconnectCursor({
  indexedDB,
  reconnectCursor,
  sessionID,
  streamIdentity,
}: {
  indexedDB: IndexedDBLike | undefined;
  reconnectCursor: FactoryEventReconnectCursor;
  sessionID: string;
  streamIdentity: StreamDerivedCacheIdentity;
}): Promise<
  | { kind: "ready"; result: DashboardSessionSyncPreflightBootstrap }
  | { kind: "recovery"; recovery: DashboardSessionRecoveryState }
  | { kind: "stale" }
  | { kind: "error"; outcome: DashboardSessionSyncPreflightOutcome }
> {
  let cursorResponse: FactorySessionSyncPreflightResponse;
  try {
    cursorResponse = await getFactorySessionSyncPreflight(
      sessionID,
      reconnectCursor,
      {
        logicalResolve: logicalResolveFromStreamIdentity(streamIdentity),
      },
    );
  } catch (error: unknown) {
    return {
      kind: "error",
      outcome: syncPreflightError(
        error,
        "The dashboard could not validate the saved replay cursor.",
      ),
    };
  }

  if (isNonRecoverableSyncPreflightReason(cursorResponse.reasonCode)) {
    await clearCheckpointForStreamIdentity(indexedDB, streamIdentity);
    return {
      kind: "recovery",
      recovery: recoveryFromResponse(cursorResponse),
    };
  }

  const cursorStreamIdentity =
    streamIdentityFromSyncPreflightResponse(cursorResponse);
  if (!cursorStreamIdentity) {
    await clearCheckpointForStreamIdentity(indexedDB, streamIdentity);
    return {
      kind: "recovery",
      recovery: recoveryFromResponse(cursorResponse),
    };
  }

  if (cursorResponse.reasonCode === "logical_session_remap") {
    await clearCheckpointForStreamIdentity(indexedDB, streamIdentity);
    return readyAfterLogicalRemap(cursorResponse, cursorStreamIdentity);
  }

  if (
    cursorResponse.reasonCode === "cursor_stale" ||
    !cursorResponse.checkpointReusable ||
    !cursorResponse.reconnectCursor.validForStreamGeneration
  ) {
    await clearCheckpointForStreamIdentity(indexedDB, streamIdentity);
    return { kind: "stale" };
  }

  return {
    kind: "ready",
    result: {
      checkpoint: await readTimelineCheckpoint(indexedDB, streamIdentity),
      reconnectCursor,
      remappedFactorySessionId: null,
      streamIdentity,
    },
  };
}

export async function bootstrapDashboardSessionSyncPreflight({
  indexedDB,
  refreshToken,
  sessionID,
}: {
  indexedDB: IndexedDBLike | undefined;
  refreshToken: number;
  sessionID: string;
}): Promise<DashboardSessionSyncPreflightOutcome> {
  if (refreshToken > 0) {
    await clearStoredTimelineCheckpointsForFactorySessionID(
      indexedDB,
      sessionID,
    );
  }

  let response: FactorySessionSyncPreflightResponse;
  try {
    response = await getFactorySessionSyncPreflight(sessionID);
  } catch (error: unknown) {
    return syncPreflightError(
      error,
      "The dashboard could not load the selected session.",
    );
  }

  if (isNonRecoverableSyncPreflightReason(response.reasonCode)) {
    const remapAttempt = await attemptStoredCheckpointLogicalRemap({
      indexedDB,
      response,
      sessionID,
    });
    if (remapAttempt.kind === "error") {
      return remapAttempt.outcome;
    }
    response = remapAttempt.response;
  }

  if (isNonRecoverableSyncPreflightReason(response.reasonCode)) {
    await clearStoredTimelineCheckpointsForFactorySessionID(
      indexedDB,
      sessionID,
    );
    return {
      kind: "recovery",
      recovery: recoveryFromResponse(response),
    };
  }

  const streamIdentity = streamIdentityFromSyncPreflightResponse(response);
  if (!streamIdentity) {
    return {
      kind: "recovery",
      recovery: recoveryFromResponse(response),
    };
  }

  if (response.reasonCode === "logical_session_remap") {
    await clearStoredTimelineCheckpointsForFactorySessionID(
      indexedDB,
      sessionID,
    );
    return readyAfterLogicalRemap(response, streamIdentity);
  }

  const checkpoint = await readTimelineCheckpoint(indexedDB, streamIdentity);
  const reconnectCursor = reconnectCursorFromCheckpoint(checkpoint);

  if (reconnectCursor) {
    const cursorOutcome = await validateSavedReconnectCursor({
      indexedDB,
      reconnectCursor,
      sessionID,
      streamIdentity,
    });
    if (cursorOutcome.kind === "error") {
      return cursorOutcome.outcome;
    }
    if (cursorOutcome.kind === "recovery" || cursorOutcome.kind === "ready") {
      return cursorOutcome;
    }
    return {
      kind: "ready",
      result: {
        checkpoint: null,
        reconnectCursor: undefined,
        remappedFactorySessionId: null,
        streamIdentity,
      },
    };
  }

  return {
    kind: "ready",
    result: {
      checkpoint,
      reconnectCursor,
      remappedFactorySessionId: null,
      streamIdentity,
    },
  };
}
