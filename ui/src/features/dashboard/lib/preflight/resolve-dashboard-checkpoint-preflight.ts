import type { FactoryEventReconnectCursor } from "../../../../api/events";
import {
  type FactorySessionSyncPreflightResponse,
  getFactorySessionSyncPreflight,
} from "../../../../api/factory-sessions";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import type {
  PersistedTimelineCheckpointPeek,
  TimelineCheckpointStreamIdentity,
} from "../../../timeline/public/checkpoint-persistence";
import {
  peekPersistedTimelineCheckpoint,
  readTimelineCheckpoint,
} from "../../../timeline/public/checkpoint-persistence";
import { reconnectCursorFromCheckpoint } from "../../../timeline/public/checkpoint-reconnect";
import type { FactoryTimelineCheckpoint } from "../../../timeline/public/store";
import { isDefaultToRuntimeSessionAliasRemap } from "../dashboard-session-lifecycle";
import {
  classifyCheckpointIdentityMismatchDetail,
  type SessionPersistenceDiagnosticDetail,
} from "../session-persistence/diagnostics";
import {
  resolveDashboardSyncPreflight,
  shouldClearCheckpointAfterPreflight,
  syncPreflightIdentityHintsFromCheckpoint,
} from "./dashboard-session-sync-preflight";

interface IndexedDBLike {
  open: IDBFactory["open"];
}

interface ResolutionDependencies {
  getSyncPreflight: typeof getFactorySessionSyncPreflight;
  peekCheckpoint: typeof peekPersistedTimelineCheckpoint;
  readCheckpoint: typeof readTimelineCheckpoint;
}

export type DashboardCheckpointLookupOutcome =
  | "checkpoint_hit"
  | "checkpoint_miss";

interface DashboardCheckpointLookupResolution {
  checkpointLookupOutcome?: DashboardCheckpointLookupOutcome;
  identityRejectionDetail?: SessionPersistenceDiagnosticDetail;
}

export type DashboardCheckpointPreflightResolution =
  DashboardCheckpointLookupResolution &
    (
      | {
          checkpoint: FactoryTimelineCheckpoint | null;
          clearRequestedSessionCheckpoint: boolean;
          checkpointToDelete: PersistedTimelineCheckpointPeek | null;
          kind: "resume";
          reconnectCursor?: FactoryEventReconnectCursor;
          staleCursorDetected: boolean;
          requestedSessionId: string;
          resolvedSessionId: string;
          streamIdentity: TimelineCheckpointStreamIdentity;
        }
      | {
          clearRequestedSessionCheckpoint: true;
          checkpointToDelete: PersistedTimelineCheckpointPeek | null;
          kind: "remap";
          requestedSessionId: string;
          resolvedSessionId: string;
          streamIdentity: TimelineCheckpointStreamIdentity;
        }
      | {
          clearRequestedSessionCheckpoint: true;
          checkpointToDelete: PersistedTimelineCheckpointPeek | null;
          kind: "recovery";
          reasonCode: string;
          requestedSessionId: string;
        }
      | {
          clearRequestedSessionCheckpoint: true;
          checkpointToDelete: null;
          error: Error;
          kind: "error";
          requestedSessionId: string;
        }
    );

function errorOutcome(
  requestedSessionId: string,
  failure: unknown,
  checkpointLookupOutcome?: DashboardCheckpointLookupOutcome,
): DashboardCheckpointPreflightResolution {
  const message =
    failure instanceof Error && failure.message.trim() !== ""
      ? failure.message
      : "The dashboard could not validate the selected session.";
  return {
    ...(checkpointLookupOutcome ? { checkpointLookupOutcome } : {}),
    clearRequestedSessionCheckpoint: true,
    checkpointToDelete: null,
    error: new Error(message),
    kind: "error",
    requestedSessionId,
  };
}

function storedIdentityMatchesResolved(
  stored: TimelineCheckpointStreamIdentity | null | undefined,
  resolved: TimelineCheckpointStreamIdentity,
): boolean {
  return (
    stored?.backendScopeID === resolved.backendScopeID &&
    stored.factorySessionID === resolved.factorySessionID &&
    stored.logicalSessionKeyID === resolved.logicalSessionKeyID &&
    stored.streamGenerationID === resolved.streamGenerationID
  );
}

export async function resolveDashboardCheckpointPreflight({
  dependencies,
  indexedDB,
  requestedSessionId,
  signal,
}: {
  dependencies?: ResolutionDependencies;
  indexedDB: IndexedDBLike | undefined;
  requestedSessionId: string;
  signal?: AbortSignal;
}): Promise<DashboardCheckpointPreflightResolution> {
  const resolutionDependencies = dependencies ?? {
    getSyncPreflight: getFactorySessionSyncPreflight,
    peekCheckpoint: peekPersistedTimelineCheckpoint,
    readCheckpoint: readTimelineCheckpoint,
  };
  let checkpointLookupOutcome: DashboardCheckpointLookupOutcome | undefined;
  try {
    const stored = await resolutionDependencies.peekCheckpoint(
      indexedDB,
      requestedSessionId,
      { signal },
    );
    signal?.throwIfAborted();
    checkpointLookupOutcome = stored ? "checkpoint_hit" : "checkpoint_miss";
    const reconnectCursor = reconnectCursorFromCheckpoint(
      stored?.checkpoint ?? null,
    );
    const response = await resolutionDependencies.getSyncPreflight(
      requestedSessionId,
      reconnectCursor,
      {
        ...syncPreflightIdentityHintsFromCheckpoint(
          stored?.checkpoint.syncIdentity,
          stored?.streamIdentity,
        ),
        signal,
      },
    );
    signal?.throwIfAborted();
    const resolution = await resolveResponse({
      dependencies: resolutionDependencies,
      indexedDB,
      response,
      signal,
      stored,
    });
    return { ...resolution, checkpointLookupOutcome };
  } catch (failure: unknown) {
    if (signal?.aborted) {
      throw failure;
    }
    return errorOutcome(requestedSessionId, failure, checkpointLookupOutcome);
  }
}

async function resolveResponse({
  dependencies,
  indexedDB,
  response,
  signal,
  stored,
}: {
  dependencies: ResolutionDependencies;
  indexedDB: IndexedDBLike | undefined;
  response: FactorySessionSyncPreflightResponse;
  signal?: AbortSignal;
  stored: PersistedTimelineCheckpointPeek | null;
}): Promise<DashboardCheckpointPreflightResolution> {
  const resolution = resolveDashboardSyncPreflight(response);
  if (resolution.kind === "non-recoverable") {
    return {
      clearRequestedSessionCheckpoint: true,
      checkpointToDelete: stored,
      kind: "recovery",
      reasonCode: resolution.recovery.reasonCode,
      requestedSessionId: resolution.recovery.requestedSessionId,
    };
  }

  const clearRequestedSessionCheckpoint =
    shouldClearCheckpointAfterPreflight(response) ||
    (stored?.streamIdentity != null &&
      !storedIdentityMatchesResolved(
        stored.streamIdentity,
        resolution.streamIdentity,
      ));
  const identityRejectionDetail = stored?.streamIdentity
    ? (classifyCheckpointIdentityMismatchDetail(
        stored.streamIdentity,
        resolution.streamIdentity,
      ) ?? undefined)
    : undefined;
  if (
    resolution.resolvedSessionId !== resolution.requestedSessionId &&
    !isDefaultToRuntimeSessionAliasRemap(
      resolution.requestedSessionId,
      resolution.resolvedSessionId,
    )
  ) {
    return {
      clearRequestedSessionCheckpoint: true,
      checkpointToDelete: stored,
      ...(identityRejectionDetail ? { identityRejectionDetail } : {}),
      kind: "remap",
      requestedSessionId: resolution.requestedSessionId,
      resolvedSessionId: resolution.resolvedSessionId,
      streamIdentity: resolution.streamIdentity,
    };
  }

  const checkpoint =
    resolution.checkpointReusable &&
    resolution.reconnectCursor &&
    !clearRequestedSessionCheckpoint
      ? await dependencies.readCheckpoint(
          indexedDB,
          resolution.streamIdentity,
          { signal },
        )
      : null;
  signal?.throwIfAborted();
  return {
    checkpoint,
    clearRequestedSessionCheckpoint,
    checkpointToDelete: clearRequestedSessionCheckpoint ? stored : null,
    ...(identityRejectionDetail ? { identityRejectionDetail } : {}),
    kind: "resume",
    reconnectCursor: clearRequestedSessionCheckpoint
      ? undefined
      : resolution.reconnectCursor,
    staleCursorDetected:
      response.reasonCode ===
      FactorySessionSyncPreflightReasonCode.cursor_stale,
    requestedSessionId: resolution.requestedSessionId,
    resolvedSessionId: resolution.resolvedSessionId,
    streamIdentity: resolution.streamIdentity,
  };
}
