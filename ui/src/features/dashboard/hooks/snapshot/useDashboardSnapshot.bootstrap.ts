import { useEffect, useState } from "react";

import {
  type FactoryTimelineCheckpoint,
  purgeLegacyTimelineCheckpoints,
  type TimelineCheckpointStreamIdentity,
} from "../../../timeline/public";
import { useSetDashboardSessionID } from "../../session/dashboard-session-provider";
import {
  bootstrapDashboardSessionSyncPreflight,
  type DashboardSessionRecoveryState,
} from "../../lib/dashboard-session-sync-preflight";
import { normalizeBackendRuntimeCacheScope } from "../../lib/backend-scope/backend-runtime-cache-scope";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";

function applyPreflightBootstrapIdentity({
  rawSessionID,
  remappedFactorySessionId,
  resolvedStreamIdentity,
  setBackendRuntimeCacheScope,
  setResolvedStreamIdentity,
  setSelectedSessionID,
  setStreamIdentity,
}: {
  rawSessionID: string;
  remappedFactorySessionId: string | null | undefined;
  resolvedStreamIdentity: TimelineCheckpointStreamIdentity;
  setBackendRuntimeCacheScope: (backendRuntimeCacheScope: string | null) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
  setSelectedSessionID: (sessionID: string) => void;
  setStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
}) {
  setStreamIdentity(resolvedStreamIdentity);
  setResolvedStreamIdentity(resolvedStreamIdentity);
  setBackendRuntimeCacheScope(
    normalizeBackendRuntimeCacheScope(resolvedStreamIdentity.backendScopeID),
  );
  if (
    remappedFactorySessionId != null &&
    remappedFactorySessionId !== rawSessionID
  ) {
    setSelectedSessionID(remappedFactorySessionId);
  }
}

async function finalizePreflightBootstrapSuccess({
  checkpoint,
  checkpointHydrationKey,
  indexedDB,
  remappedFactorySessionId,
  resolvedStreamIdentity,
  restoreCheckpoint,
  setBackendRuntimeCacheScope,
  setCheckpointHydratedKey,
  setPersistedCheckpoint,
  setPreflightReadyKey,
  setResolvedStreamIdentity,
  setSelectedSessionID,
  setStreamIdentity,
  rawSessionID,
}: {
  checkpoint: FactoryTimelineCheckpoint | null;
  checkpointHydrationKey: string;
  indexedDB: IDBFactory;
  remappedFactorySessionId: string | null | undefined;
  resolvedStreamIdentity: TimelineCheckpointStreamIdentity;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  setBackendRuntimeCacheScope: (backendRuntimeCacheScope: string | null) => void;
  setCheckpointHydratedKey: (key: string) => void;
  setPersistedCheckpoint: (checkpoint: FactoryTimelineCheckpoint | null) => void;
  setPreflightReadyKey: (key: string) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
  setSelectedSessionID: (sessionID: string) => void;
  setStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
  rawSessionID: string;
}) {
  applyPreflightBootstrapIdentity({
    rawSessionID,
    remappedFactorySessionId,
    resolvedStreamIdentity,
    setBackendRuntimeCacheScope,
    setResolvedStreamIdentity,
    setSelectedSessionID,
    setStreamIdentity,
  });
  await purgeLegacyTimelineCheckpoints(indexedDB);
  setPreflightReadyKey(checkpointHydrationKey);
  if (checkpoint) {
    restoreCheckpoint(checkpoint);
  }
  setPersistedCheckpoint(checkpoint);
  setCheckpointHydratedKey(checkpointHydrationKey);
}

function resetGuardedTimelineCheckpointBootstrap({
  setBackendRuntimeCacheScope,
  setCheckpointHydratedKey,
  setPersistedCheckpoint,
  setPreflightError,
  setPreflightReadyKey,
  setPreflightRecovery,
  setResolvedStreamIdentity,
  setStreamIdentity,
}: {
  setBackendRuntimeCacheScope: (backendRuntimeCacheScope: string | null) => void;
  setCheckpointHydratedKey: (key: string | null) => void;
  setPersistedCheckpoint: (checkpoint: FactoryTimelineCheckpoint | null) => void;
  setPreflightError: (error: Error | null) => void;
  setPreflightReadyKey: (key: string | null) => void;
  setPreflightRecovery: (recovery: DashboardSessionRecoveryState | null) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
  setStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
}) {
  setCheckpointHydratedKey(null);
  setPersistedCheckpoint(null);
  setPreflightError(null);
  setPreflightRecovery(null);
  setPreflightReadyKey(null);
  setStreamIdentity(null);
  setResolvedStreamIdentity(null);
  setBackendRuntimeCacheScope(null);
}

async function handlePreflightBootstrapOutcome({
  cancelled,
  checkpointHydrationKey,
  indexedDB,
  outcome,
  rawSessionID,
  restoreCheckpoint,
  setBackendRuntimeCacheScope,
  setCheckpointHydratedKey,
  setPersistedCheckpoint,
  setPreflightError,
  setPreflightReadyKey,
  setPreflightRecovery,
  setResolvedStreamIdentity,
  setSelectedSessionID,
  setStreamIdentity,
  setStreamState,
}: {
  cancelled: () => boolean;
  checkpointHydrationKey: string;
  indexedDB: IDBFactory;
  outcome: Awaited<ReturnType<typeof bootstrapDashboardSessionSyncPreflight>>;
  rawSessionID: string;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  setBackendRuntimeCacheScope: (backendRuntimeCacheScope: string | null) => void;
  setCheckpointHydratedKey: (key: string) => void;
  setPersistedCheckpoint: (checkpoint: FactoryTimelineCheckpoint | null) => void;
  setPreflightError: (error: Error | null) => void;
  setPreflightReadyKey: (key: string) => void;
  setPreflightRecovery: (recovery: DashboardSessionRecoveryState | null) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
  setSelectedSessionID: (sessionID: string) => void;
  setStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
  setStreamState: ReturnType<
    typeof useDashboardStreamStore.getState
  >["setStreamState"];
}) {
  if (cancelled()) {
    return;
  }
  if (outcome.kind === "error") {
    setPreflightError(outcome.error);
    setStreamState({
      message: outcome.error.message,
      status: "offline",
    });
    setCheckpointHydratedKey(checkpointHydrationKey);
    return;
  }
  if (outcome.kind === "recovery") {
    setPreflightRecovery(outcome.recovery);
    setCheckpointHydratedKey(checkpointHydrationKey);
    return;
  }

  const {
    checkpoint,
    reconnectCursor: _reconnectCursor,
    remappedFactorySessionId,
    streamIdentity: resolvedStreamIdentity,
  } = outcome.result;
  await finalizePreflightBootstrapSuccess({
    checkpoint,
    checkpointHydrationKey,
    indexedDB,
    rawSessionID,
    remappedFactorySessionId,
    resolvedStreamIdentity,
    restoreCheckpoint,
    setBackendRuntimeCacheScope,
    setCheckpointHydratedKey,
    setPersistedCheckpoint,
    setPreflightReadyKey,
    setResolvedStreamIdentity,
    setSelectedSessionID,
    setStreamIdentity,
  });
}

export function useGuardedTimelineCheckpointBootstrap({
  checkpointHydrationKey,
  checkpointsDisabled,
  rawSessionID,
  refreshToken,
  restoreCheckpoint,
  setBackendRuntimeCacheScope,
  setResolvedStreamIdentity,
}: {
  checkpointHydrationKey: string | null;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  refreshToken: number;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  setBackendRuntimeCacheScope: (backendRuntimeCacheScope: string | null) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
}) {
  const setSelectedSessionID = useSetDashboardSessionID();
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
  const [checkpointHydratedKey, setCheckpointHydratedKey] =
    useState<string | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [preflightReadyKey, setPreflightReadyKey] = useState<string | null>(
    null,
  );
  const [preflightError, setPreflightError] = useState<Error | null>(null);
  const [preflightRecovery, setPreflightRecovery] =
    useState<DashboardSessionRecoveryState | null>(null);
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);

  const checkpointHydrated = checkpointHydratedKey === checkpointHydrationKey;
  const preflightReady = preflightReadyKey === checkpointHydrationKey;

  useEffect(() => {
    let cancelled = false;

    resetGuardedTimelineCheckpointBootstrap({
      setBackendRuntimeCacheScope,
      setCheckpointHydratedKey,
      setPersistedCheckpoint,
      setPreflightError,
      setPreflightReadyKey,
      setPreflightRecovery,
      setResolvedStreamIdentity,
      setStreamIdentity,
    });

    if (
      checkpointHydrationKey == null ||
      !rawSessionID ||
      typeof window === "undefined" ||
      checkpointsDisabled
    ) {
      setPreflightReadyKey(checkpointHydrationKey);
      setCheckpointHydratedKey(checkpointHydrationKey);
      return;
    }

    void bootstrapDashboardSessionSyncPreflight({
      indexedDB: window.indexedDB,
      refreshToken,
      sessionID: rawSessionID,
    })
      .then((outcome) =>
        handlePreflightBootstrapOutcome({
          cancelled: () => cancelled,
          checkpointHydrationKey,
          indexedDB: window.indexedDB,
          outcome,
          rawSessionID,
          restoreCheckpoint,
          setBackendRuntimeCacheScope,
          setCheckpointHydratedKey,
          setPersistedCheckpoint,
          setPreflightError,
          setPreflightReadyKey,
          setPreflightRecovery,
          setResolvedStreamIdentity,
          setSelectedSessionID,
          setStreamIdentity,
          setStreamState,
        }),
      )
      .catch((preflightError: unknown) => {
        if (cancelled) {
          return;
        }
        const message =
          preflightError instanceof Error && preflightError.message.trim() !== ""
            ? preflightError.message
            : "The dashboard could not load the selected session.";
        setPreflightError(new Error(message));
        setStreamState({
          message,
          status: "offline",
        });
        setCheckpointHydratedKey(checkpointHydrationKey);
      });

    return () => {
      cancelled = true;
    };
  }, [
    checkpointHydrationKey,
    checkpointsDisabled,
    rawSessionID,
    refreshToken,
    restoreCheckpoint,
    setBackendRuntimeCacheScope,
    setResolvedStreamIdentity,
    setSelectedSessionID,
    setStreamState,
  ]);

  return {
    checkpointHydrated,
    preflightError,
    preflightRecovery,
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  };
}
