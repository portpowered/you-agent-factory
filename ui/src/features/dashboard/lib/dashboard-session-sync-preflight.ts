import type { FactoryEventReconnectCursor } from "../../../api/events";
import {
  type FactorySessionLogicalResolveHint,
  type FactorySessionSyncPreflightResponse,
  getFactorySessionSyncPreflight,
} from "../../../api/factory-sessions/sync-preflight";
import {
  clearTimelineCheckpoint,
  clearStoredTimelineCheckpointsForFactorySessionID,
  type FactoryTimelineCheckpoint,
  findStoredCheckpointEnvelopeByFactorySessionID,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timeline/public";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
} from "../../timeline/lib/stream-derived-cache-identity";

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

async function clearCheckpointForStreamIdentity(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<void> {
  if (!indexedDB || streamIdentity == null) {
    return;
  }
  await clearTimelineCheckpoint(indexedDB, streamIdentity);
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
    const message =
      error instanceof Error && error.message.trim() !== ""
        ? error.message
        : "The dashboard could not load the selected session.";
    return { kind: "error", error: new Error(message) };
  }

  if (isNonRecoverableSyncPreflightReason(response.reasonCode)) {
    const storedEnvelope = await findStoredCheckpointEnvelopeByFactorySessionID(
      indexedDB,
      sessionID,
    );
    if (storedEnvelope && response.reasonCode === "session_not_found") {
      const candidateCursor = reconnectCursorFromCheckpoint(
        storedEnvelope.checkpoint,
      );
      try {
        response = await getFactorySessionSyncPreflight(
          sessionID,
          candidateCursor,
          {
            logicalResolve: {
              backendScopeID: storedEnvelope.streamIdentity.backendScopeID,
              logicalSessionKeyID:
                storedEnvelope.streamIdentity.logicalSessionKeyID,
            },
          },
        );
      } catch (error: unknown) {
        const message =
          error instanceof Error && error.message.trim() !== ""
            ? error.message
            : "The dashboard could not load the selected session.";
        return { kind: "error", error: new Error(message) };
      }
    }
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

  let checkpoint = await readTimelineCheckpoint(indexedDB, streamIdentity);
  let reconnectCursor = reconnectCursorFromCheckpoint(checkpoint);

  if (reconnectCursor) {
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
      const message =
        error instanceof Error && error.message.trim() !== ""
          ? error.message
          : "The dashboard could not validate the saved replay cursor.";
      return { kind: "error", error: new Error(message) };
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
      return {
        kind: "ready",
        result: {
          checkpoint: null,
          reconnectCursor: undefined,
          remappedFactorySessionId: cursorResponse.factorySessionId ?? null,
          streamIdentity: cursorStreamIdentity,
        },
      };
    }

    if (
      cursorResponse.reasonCode === "cursor_stale" ||
      !cursorResponse.checkpointReusable ||
      !cursorResponse.reconnectCursor.validForStreamGeneration
    ) {
      await clearCheckpointForStreamIdentity(indexedDB, streamIdentity);
      checkpoint = null;
      reconnectCursor = undefined;
    }
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
