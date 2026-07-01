import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { getFactorySessionSyncPreflight } from "../../../api/factory-sessions";
import type { FactoryTimelineCheckpoint } from "../../timeline/state/timeline/storeState";
import {
  clearTimelineCheckpointsForSession,
  peekPersistedTimelineCheckpoint,
  readTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timeline/state/timelineCheckpointPersistence";
import {
  checkpointSyncIdentityFromPreflight,
  type DashboardSessionRecoveryState,
  resolveDashboardSyncPreflight,
  shouldClearCheckpointAfterPreflight,
  shouldRemapDashboardSession,
  syncPreflightIdentityHintsFromCheckpoint,
} from "../lib/dashboard-session-sync-preflight";
import {
  clearDashboardSessionRuntimeQueries,
  recoverDashboardSessionScopedState,
} from "../lib/dashboard-session-lifecycle";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";

export interface UseDashboardCheckpointPreflightResult {
  checkpointHydrated: boolean;
  initialReconnectCursor?: ReturnType<typeof reconnectCursorFromCheckpoint>;
  preflightError: Error | null;
  preflightRecovery: DashboardSessionRecoveryState | null;
  preflightReady: boolean;
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  resolvedSessionID: string | null;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}

export function useDashboardCheckpointPreflight({
  checkpointHydrationKey,
  checkpointsDisabled,
  rawSessionID,
  refreshToken,
  restoreCheckpoint,
}: {
  checkpointHydrationKey: string | null;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  refreshToken: number;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
}): UseDashboardCheckpointPreflightResult {
  const queryClient = useQueryClient();
  const setSelectedSessionID = useDashboardSessionStore(
    (state) => state.setSelectedSessionID,
  );
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
  const [checkpointHydratedKey, setCheckpointHydratedKey] =
    useState<string | null>(null);
  const [preflightReadyKey, setPreflightReadyKey] = useState<string | null>(
    null,
  );
  const [preflightError, setPreflightError] = useState<Error | null>(null);
  const [preflightRecovery, setPreflightRecovery] =
    useState<DashboardSessionRecoveryState | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [initialReconnectCursor, setInitialReconnectCursor] = useState<
    ReturnType<typeof reconnectCursorFromCheckpoint> | undefined
  >(undefined);
  const [resolvedSessionID, setResolvedSessionID] = useState<string | null>(
    null,
  );
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);

  const checkpointHydrated = checkpointHydratedKey === checkpointHydrationKey;
  const preflightReady = preflightReadyKey === checkpointHydrationKey;

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedKey(null);
    setPersistedCheckpoint(null);
    setPreflightError(null);
    setPreflightRecovery(null);
    setPreflightReadyKey(null);
    setInitialReconnectCursor(undefined);
    setResolvedSessionID(null);
    setStreamIdentity(null);

    if (
      checkpointHydrationKey == null ||
      !rawSessionID ||
      typeof window === "undefined" ||
      checkpointsDisabled
    ) {
      setResolvedSessionID(rawSessionID);
      setPreflightReadyKey(checkpointHydrationKey);
      setCheckpointHydratedKey(checkpointHydrationKey);
      return;
    }

    void (async () => {
      const peekedCheckpoint = await peekPersistedTimelineCheckpoint(
        window.indexedDB,
        rawSessionID,
      );
      if (cancelled) {
        return;
      }

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
        if (cancelled) {
          return;
        }

        const resolution = resolveDashboardSyncPreflight(response);
        if (resolution.kind === "non-recoverable") {
          await clearTimelineCheckpointsForSession(
            window.indexedDB,
            rawSessionID,
          );
          if (cancelled) {
            return;
          }
          recoverDashboardSessionScopedState(
            queryClient,
            rawSessionID,
            () => {},
          );
          setPreflightRecovery(resolution.recovery);
          setPreflightReadyKey(checkpointHydrationKey);
          setCheckpointHydratedKey(checkpointHydrationKey);
          return;
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
          streamGenerationID: resolvedStreamIdentity.streamGenerationID,
        };

        if (
          shouldRemapDashboardSession(response, requestedSessionId) ||
          shouldClearCheckpointAfterPreflight(response)
        ) {
          await clearTimelineCheckpointsForSession(
            window.indexedDB,
            requestedSessionId,
          );
          if (cancelled) {
            return;
          }
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
        if (checkpointReusable && validatedReconnectCursor) {
          checkpoint = await readTimelineCheckpoint(
            window.indexedDB,
            resolvedSessionId,
            checkpointStreamIdentity,
          );
          if (cancelled) {
            return;
          }
        }

        if (checkpoint) {
          restoreCheckpoint({
            ...checkpoint,
            syncIdentity: checkpointSyncIdentityFromPreflight(
              resolvedStreamIdentity,
            ),
          });
        }

        if (resolvedSessionId !== requestedSessionId) {
          setSelectedSessionID(resolvedSessionId);
        }

        setResolvedSessionID(resolvedSessionId);
        setStreamIdentity(checkpointStreamIdentity);
        setInitialReconnectCursor(validatedReconnectCursor);
        setPersistedCheckpoint(checkpoint);
        setPreflightReadyKey(checkpointHydrationKey);
        setCheckpointHydratedKey(checkpointHydrationKey);
      } catch (preflightFailure: unknown) {
        if (cancelled) {
          return;
        }
        await clearTimelineCheckpointsForSession(
          window.indexedDB,
          rawSessionID,
        );
        if (cancelled) {
          return;
        }
        const message =
          preflightFailure instanceof Error &&
          preflightFailure.message.trim() !== ""
            ? preflightFailure.message
            : "The dashboard could not validate the selected session.";
        setPreflightError(new Error(message));
        setStreamState({
          message,
          status: "offline",
        });
        setCheckpointHydratedKey(checkpointHydrationKey);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [
    checkpointHydrationKey,
    checkpointsDisabled,
    queryClient,
    rawSessionID,
    refreshToken,
    restoreCheckpoint,
    setSelectedSessionID,
    setStreamState,
  ]);

  return {
    checkpointHydrated,
    initialReconnectCursor,
    preflightError,
    preflightRecovery,
    preflightReady,
    persistedCheckpoint,
    resolvedSessionID,
    streamIdentity,
  };
}
