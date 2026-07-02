import type { QueryClient } from "@tanstack/react-query";

import { getFactorySessionSyncPreflight } from "../../../../api/factory-sessions";
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
  if (normalizedStored.factorySessionID !== normalizedResolved.factorySessionID) {
    return true;
  }
  return (
    normalizedStored.backendScopeID === normalizedResolved.backendScopeID &&
    normalizedStored.logicalSessionKeyID ===
      normalizedResolved.logicalSessionKeyID &&
    normalizedStored.streamGenerationID === normalizedResolved.streamGenerationID
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

export async function runDashboardCheckpointPreflight({
  onRemapSessionID,
  onStreamOffline,
  queryClient,
  rawSessionID,
  restoreCheckpoint,
}: {
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
      await clearTimelineCheckpointsForSession(window.indexedDB, rawSessionID);
      recoverDashboardSessionScopedState(queryClient, rawSessionID, () => {});
      return {
        initialReconnectCursor: undefined,
        persistedCheckpoint: null,
        preflightError: null,
        preflightRecovery: resolution.recovery,
        resolvedSessionID: null,
        streamIdentity: null,
      };
    }

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

    let reconnectCursorForStream = validatedReconnectCursor;
    if (
      peekedCheckpoint?.streamIdentity != null &&
      !storedCheckpointMatchesResolvedStream(
        peekedCheckpoint.streamIdentity,
        checkpointStreamIdentity,
      )
    ) {
      await clearTimelineCheckpointsForSession(
        window.indexedDB,
        requestedSessionId,
      );
      recoverDashboardSessionScopedState(
        queryClient,
        requestedSessionId,
        () => {},
      );
      reconnectCursorForStream = undefined;
    }

    if (shouldClearCheckpointAfterPreflight(response)) {
      await clearTimelineCheckpointsForSession(
        window.indexedDB,
        requestedSessionId,
      );
      recoverDashboardSessionScopedState(
        queryClient,
        requestedSessionId,
        () => {},
      );
      if (resolvedSessionId !== requestedSessionId) {
        clearDashboardSessionRuntimeQueries(
          queryClient,
          requestedSessionId,
        );
      }
    }

    let checkpoint: FactoryTimelineCheckpoint | null = null;
    if (checkpointReusable && reconnectCursorForStream) {
      checkpoint = await readTimelineCheckpoint(
        window.indexedDB,
        checkpointStreamIdentity,
      );
    }

    if (checkpoint) {
      restoreCheckpoint({
        ...checkpoint,
        syncIdentity: checkpointSyncIdentityFromPreflight(
          resolvedStreamIdentity,
        ),
      });
    }

    if (
      resolvedSessionId !== requestedSessionId &&
      !isDefaultToRuntimeSessionAliasRemap(
        requestedSessionId,
        resolvedSessionId,
      )
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
  } catch (preflightFailure: unknown) {
    await clearTimelineCheckpointsForSession(window.indexedDB, rawSessionID);
    const message =
      preflightFailure instanceof Error &&
      preflightFailure.message.trim() !== ""
        ? preflightFailure.message
        : "The dashboard could not validate the selected session.";
    onStreamOffline(message);
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
