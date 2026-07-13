import type { FactoryEventReconnectCursor } from "../../../../api/events";
import {
  type FactorySessionSyncPreflightResponse,
  getFactorySessionSyncPreflight,
} from "../../../../api/factory-sessions";
import type {
  FactoryTimelineCheckpoint,
  PersistedTimelineCheckpointPeek,
  TimelineCheckpointStreamIdentity,
} from "../../../timeline/public";
import {
  peekPersistedTimelineCheckpoint,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
} from "../../../timeline/public";
import { isDefaultToRuntimeSessionAliasRemap } from "../dashboard-session-lifecycle";
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

export type DashboardCheckpointPreflightResolution =
  | {
      checkpoint: FactoryTimelineCheckpoint | null;
      clearRequestedSessionCheckpoint: boolean;
      checkpointToDelete: PersistedTimelineCheckpointPeek | null;
      kind: "resume";
      reconnectCursor?: FactoryEventReconnectCursor;
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
    };

function errorOutcome(
  requestedSessionId: string,
  failure: unknown,
): DashboardCheckpointPreflightResolution {
  const message =
    failure instanceof Error && failure.message.trim() !== ""
      ? failure.message
      : "The dashboard could not validate the selected session.";
  return {
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
  try {
    const stored = await resolutionDependencies.peekCheckpoint(
      indexedDB,
      requestedSessionId,
      { signal },
    );
    signal?.throwIfAborted();
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
    return resolveResponse({
      dependencies: resolutionDependencies,
      indexedDB,
      response,
      signal,
      stored,
    });
  } catch (failure: unknown) {
    if (signal?.aborted) {
      throw failure;
    }
    return errorOutcome(requestedSessionId, failure);
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
    kind: "resume",
    reconnectCursor: clearRequestedSessionCheckpoint
      ? undefined
      : resolution.reconnectCursor,
    requestedSessionId: resolution.requestedSessionId,
    resolvedSessionId: resolution.resolvedSessionId,
    streamIdentity: resolution.streamIdentity,
  };
}
