import type { QueryClient } from "@tanstack/react-query";
import { getFactorySessionSyncPreflight } from "../../../../api/factory-sessions";
import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import type { FactoryTimelineCheckpoint } from "../../../timeline/public";
import {
  clearTimelineCheckpointsForSession,
  normalizeStreamDerivedCacheIdentity,
  peekPersistedTimelineCheckpoint,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../../timeline/public";
import {
  clearDashboardSessionRuntimeQueries,
  isDefaultToRuntimeSessionAliasRemap,
  recoverDashboardSessionScopedState,
} from "../dashboard-session-lifecycle";
import {
  checkpointSyncIdentityFromPreflight,
  type DashboardSessionRecoveryState,
  type DashboardSyncPreflightResolution,
  resolveDashboardSyncPreflight,
  shouldClearCheckpointAfterPreflight,
  syncPreflightIdentityHintsFromCheckpoint,
} from "./dashboard-session-sync-preflight";

function storedCheckpointMatchesResolvedStream(
  stored: TimelineCheckpointStreamIdentity | null | undefined,
  resolved: TimelineCheckpointStreamIdentity,
): boolean {
  const normalizedStored = normalizeStreamDerivedCacheIdentity(stored);
  const normalizedResolved = normalizeStreamDerivedCacheIdentity(resolved);
  if (!normalizedStored || !normalizedResolved) {
    return true;
  }
  if (
    normalizedStored.factorySessionID !== normalizedResolved.factorySessionID
  ) {
    return true;
  }
  return (
    normalizedStored.backendScopeID === normalizedResolved.backendScopeID &&
    normalizedStored.logicalSessionKeyID ===
      normalizedResolved.logicalSessionKeyID &&
    normalizedStored.streamGenerationID ===
      normalizedResolved.streamGenerationID
  );
}

export interface DashboardCheckpointPreflightHydration {
  initialReconnectCursor?: ReturnType<typeof reconnectCursorFromCheckpoint>;
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  preflightError: Error | null;
  preflightRecovery: DashboardSessionRecoveryState | null;
  resolvedSessionID: string | null;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}

type ResumePreflightResolution = Extract<
  DashboardSyncPreflightResolution,
  { kind: "resume" }
>;

async function clearRequestedSessionCheckpoint(
  queryClient: QueryClient,
  requestedSessionId: string,
  isCurrent: () => boolean,
): Promise<void> {
  if (!isCurrent()) {
    return;
  }
  await clearTimelineCheckpointsForSession(
    window.indexedDB,
    requestedSessionId,
  );
  if (!isCurrent()) {
    return;
  }
  recoverDashboardSessionScopedState(queryClient, requestedSessionId, () => {});
}

async function reconcileReconnectCursor({
  peekedStreamIdentity,
  isCurrent,
  queryClient,
  requestedSessionId,
  resolvedStreamIdentity,
  validatedReconnectCursor,
}: {
  peekedStreamIdentity: TimelineCheckpointStreamIdentity | null | undefined;
  isCurrent: () => boolean;
  queryClient: QueryClient;
  requestedSessionId: string;
  resolvedStreamIdentity: TimelineCheckpointStreamIdentity;
  validatedReconnectCursor: ResumePreflightResolution["reconnectCursor"];
}): Promise<ResumePreflightResolution["reconnectCursor"]> {
  if (
    peekedStreamIdentity != null &&
    !storedCheckpointMatchesResolvedStream(
      peekedStreamIdentity,
      resolvedStreamIdentity,
    )
  ) {
    await clearRequestedSessionCheckpoint(
      queryClient,
      requestedSessionId,
      isCurrent,
    );
    return undefined;
  }
  return validatedReconnectCursor;
}

async function loadRestoredCheckpoint({
  checkpointReusable,
  checkpointStreamIdentity,
  isCurrent,
  queryClient,
  reconnectCursorForStream,
  requestedSessionId,
  resolvedSessionId,
  resolvedStreamIdentity,
  response,
  restoreCheckpoint,
}: {
  checkpointReusable: boolean;
  checkpointStreamIdentity: TimelineCheckpointStreamIdentity;
  isCurrent: () => boolean;
  queryClient: QueryClient;
  reconnectCursorForStream: ResumePreflightResolution["reconnectCursor"];
  requestedSessionId: string;
  resolvedSessionId: string;
  resolvedStreamIdentity: TimelineCheckpointStreamIdentity;
  response: FactorySessionSyncPreflightResponse;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}): Promise<FactoryTimelineCheckpoint | null> {
  if (shouldClearCheckpointAfterPreflight(response)) {
    await clearRequestedSessionCheckpoint(
      queryClient,
      requestedSessionId,
      isCurrent,
    );
    if (isCurrent() && resolvedSessionId !== requestedSessionId) {
      clearDashboardSessionRuntimeQueries(queryClient, requestedSessionId);
    }
  }

  let checkpoint: FactoryTimelineCheckpoint | null = null;
  if (isCurrent() && checkpointReusable && reconnectCursorForStream) {
    checkpoint = await readTimelineCheckpoint(
      window.indexedDB,
      checkpointStreamIdentity,
    );
  }

  if (isCurrent() && checkpoint) {
    restoreCheckpoint({
      ...checkpoint,
      syncIdentity: checkpointSyncIdentityFromPreflight(resolvedStreamIdentity),
    });
  }

  return checkpoint;
}

async function hydrateResumePreflight({
  isCurrent,
  onRemapSessionID,
  peekedCheckpoint,
  queryClient,
  resolution,
  response,
  restoreCheckpoint,
}: {
  isCurrent: () => boolean;
  onRemapSessionID: (sessionID: string) => void;
  peekedCheckpoint: Awaited<ReturnType<typeof peekPersistedTimelineCheckpoint>>;
  queryClient: QueryClient;
  resolution: ResumePreflightResolution;
  response: FactorySessionSyncPreflightResponse;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}): Promise<DashboardCheckpointPreflightHydration> {
  const {
    checkpointReusable,
    reconnectCursor: validatedReconnectCursor,
    requestedSessionId,
    resolvedSessionId,
    streamIdentity: resolvedStreamIdentity,
  } = resolution;

  const checkpointStreamIdentity: TimelineCheckpointStreamIdentity = {
    backendScopeID: resolvedStreamIdentity.backendScopeID,
    factorySessionID: resolvedStreamIdentity.factorySessionID,
    logicalSessionKeyID: resolvedStreamIdentity.logicalSessionKeyID,
    streamGenerationID: resolvedStreamIdentity.streamGenerationID,
  };

  const reconnectCursorForStream = await reconcileReconnectCursor({
    isCurrent,
    peekedStreamIdentity: peekedCheckpoint?.streamIdentity,
    queryClient,
    requestedSessionId,
    resolvedStreamIdentity: checkpointStreamIdentity,
    validatedReconnectCursor,
  });

  const checkpoint = await loadRestoredCheckpoint({
    checkpointReusable,
    checkpointStreamIdentity,
    isCurrent,
    queryClient,
    reconnectCursorForStream,
    requestedSessionId,
    resolvedSessionId,
    resolvedStreamIdentity: checkpointStreamIdentity,
    response,
    restoreCheckpoint,
  });

  if (
    isCurrent() &&
    resolvedSessionId !== requestedSessionId &&
    !isDefaultToRuntimeSessionAliasRemap(requestedSessionId, resolvedSessionId)
  ) {
    onRemapSessionID(resolvedSessionId);
  }

  return {
    initialReconnectCursor: reconnectCursorForStream,
    persistedCheckpoint: checkpoint,
    preflightError: null,
    preflightRecovery: null,
    resolvedSessionID: resolvedSessionId,
    streamIdentity: checkpointStreamIdentity,
  };
}

export async function runDashboardCheckpointPreflight({
  isCurrent,
  onRemapSessionID,
  onStreamOffline,
  queryClient,
  rawSessionID,
  restoreCheckpoint,
}: {
  isCurrent: () => boolean;
  onRemapSessionID: (sessionID: string) => void;
  onStreamOffline: (message: string) => void;
  queryClient: QueryClient;
  rawSessionID: string;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}): Promise<DashboardCheckpointPreflightHydration> {
  const peekedCheckpoint = await peekPersistedTimelineCheckpoint(
    window.indexedDB,
    rawSessionID,
  );

  const reconnectCursor = reconnectCursorFromCheckpoint(
    peekedCheckpoint?.checkpoint ?? null,
  );
  const identityHints = syncPreflightIdentityHintsFromCheckpoint(
    peekedCheckpoint?.checkpoint.syncIdentity,
    peekedCheckpoint?.streamIdentity,
  );

  try {
    const response = await getFactorySessionSyncPreflight(
      rawSessionID,
      reconnectCursor,
      identityHints,
    );

    const resolution = resolveDashboardSyncPreflight(response);
    if (resolution.kind === "non-recoverable") {
      if (!isCurrent()) {
        return {
          initialReconnectCursor: undefined,
          persistedCheckpoint: null,
          preflightError: null,
          preflightRecovery: resolution.recovery,
          resolvedSessionID: null,
          streamIdentity: null,
        };
      }
      await clearTimelineCheckpointsForSession(window.indexedDB, rawSessionID);
      if (isCurrent()) {
        recoverDashboardSessionScopedState(queryClient, rawSessionID, () => {});
      }
      return {
        initialReconnectCursor: undefined,
        persistedCheckpoint: null,
        preflightError: null,
        preflightRecovery: resolution.recovery,
        resolvedSessionID: null,
        streamIdentity: null,
      };
    }

    return hydrateResumePreflight({
      isCurrent,
      onRemapSessionID,
      peekedCheckpoint,
      queryClient,
      resolution,
      response,
      restoreCheckpoint,
    });
  } catch (preflightFailure: unknown) {
    const message =
      preflightFailure instanceof Error &&
      preflightFailure.message.trim() !== ""
        ? preflightFailure.message
        : "The dashboard could not validate the selected session.";
    if (isCurrent()) {
      await clearTimelineCheckpointsForSession(window.indexedDB, rawSessionID);
    }
    if (isCurrent()) {
      onStreamOffline(message);
    }
    return {
      initialReconnectCursor: undefined,
      persistedCheckpoint: null,
      preflightError: new Error(message),
      preflightRecovery: null,
      resolvedSessionID: null,
      streamIdentity: null,
    };
  }
}
