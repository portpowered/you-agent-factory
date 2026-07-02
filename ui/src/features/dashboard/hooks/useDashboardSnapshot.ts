import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryEvent } from "../../../api/events";
import {
  recordSessionPersistenceInvalidation,
  silentReplayRecoveryDiagnostic,
} from "../public";
import {
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  persistTimelineCheckpoint,
  readFactoryTimelineDebugOptions,
  reconnectCursorFromCheckpoint,
  type TimelineCheckpointStreamIdentity,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardCheckpointPreflight } from "./preflight/use-dashboard-checkpoint-preflight";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

type DashboardPreflightStatus = "loading" | "non-recoverable" | "success";

function usePersistedTimelineCheckpoint({
  checkpoint,
  checkpointsDisabled,
  streamIdentity,
  syncIdentity,
}: {
  checkpoint: FactoryTimelineCheckpoint | undefined;
  checkpointsDisabled: boolean;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
  syncIdentity?: FactoryTimelineCheckpoint["syncIdentity"];
}) {
  useEffect(() => {
    if (typeof window === "undefined" || checkpointsDisabled) {
      return;
    }
    const persistHandle = window.setTimeout(() => {
      void persistTimelineCheckpoint(
        window.indexedDB,
        checkpoint
          ? {
              ...checkpoint,
              ...(syncIdentity ? { syncIdentity } : {}),
            }
          : undefined,
        streamIdentity,
      );
    }, 750);
    return () => {
      window.clearTimeout(persistHandle);
    };
  }, [checkpoint, checkpointsDisabled, streamIdentity, syncIdentity]);
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: snapshot composition keeps preflight, checkpoint hydration, and stream wiring in one hook.
export function useDashboardSnapshot({
  locale,
  refreshToken = 0,
}: UseDashboardSnapshotOptions = {}) {
  const appendEvents = useFactoryTimelineStore((state) => state.appendEvents);
  const currentReplayCheckpoint = useFactoryTimelineStore(
    (state) => state.currentReplayCheckpoint,
  );
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const restoreCheckpoint = useFactoryTimelineStore(
    (state) => state.restoreCheckpoint,
  );
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const { error, isInitialLoading, snapshot, streamState } =
    useDashboardWorldView();
  const { isPaused, rawSessionID } = useDashboardSession();
  const debugOptions = useMemo(() => readFactoryTimelineDebugOptions(), []);
  const checkpointHydrationKey = useMemo(
    () => (rawSessionID == null ? null : `${rawSessionID}::${refreshToken}`),
    [rawSessionID, refreshToken],
  );
  const reconnectCursorInvalidatedRef = useRef(false);
  const [reconnectCursorRevision, setReconnectCursorRevision] = useState(0);
  const defaultRuntimeSessionIDRef = useRef<string | null>(null);
  const lastPersistedCheckpointRef =
    useRef<FactoryTimelineCheckpoint | null>(null);
  const lastSessionIDRef = useRef<string | null>(null);
  const queuedAppendRef =
    useRef<(events: FactoryEvent[]) => void>(appendEvents);

  queuedAppendRef.current = appendEvents;

  useDashboardSessionLifecycle({
    locale,
    refreshToken,
    sessionID: rawSessionID,
  });

  const {
    checkpointHydrated,
    initialReconnectCursor: preflightReconnectCursor,
    preflightError,
    preflightRecovery,
    persistedCheckpoint,
    preflightReady,
    resolvedSessionID,
    streamIdentity,
  } = useDashboardCheckpointPreflight({
    checkpointHydrationKey,
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    rawSessionID,
    refreshToken,
    restoreCheckpoint,
  });

  const effectiveSessionID = resolvedSessionID ?? rawSessionID;

  if (
    resolvedSessionID != null &&
    rawSessionID != null &&
    rawSessionID === DEFAULT_FACTORY_SESSION_ID
  ) {
    defaultRuntimeSessionIDRef.current = resolvedSessionID;
  }

  useEffect(() => {
    if (lastSessionIDRef.current === effectiveSessionID) {
      return;
    }
    const previousSessionID = lastSessionIDRef.current;
    lastSessionIDRef.current = effectiveSessionID;
    const runtimeUUID = defaultRuntimeSessionIDRef.current;
    const isDefaultRuntimeIdentityTransition =
      runtimeUUID != null &&
      previousSessionID != null &&
      ((previousSessionID === DEFAULT_FACTORY_SESSION_ID &&
        effectiveSessionID === runtimeUUID) ||
        (previousSessionID === runtimeUUID &&
          effectiveSessionID === DEFAULT_FACTORY_SESSION_ID));
    if (!isDefaultRuntimeIdentityTransition) {
      reconnectCursorInvalidatedRef.current = false;
      setReconnectCursorRevision((revision) => revision + 1);
    }
  }, [effectiveSessionID]);

  lastPersistedCheckpointRef.current = persistedCheckpoint;

  // biome-ignore lint/correctness/useExhaustiveDependencies: refreshToken intentionally retriggers invalidated cursor reads on manual stream retry.
  const initialReconnectCursor = useMemo(
    () =>
      reconnectCursorInvalidatedRef.current
        ? undefined
        : (preflightReconnectCursor ??
          reconnectCursorFromCheckpoint(persistedCheckpoint)),
    [persistedCheckpoint, preflightReconnectCursor, reconnectCursorRevision, refreshToken],
  );

  const handleInvalidReconnectCursor = useCallback(() => {
    reconnectCursorInvalidatedRef.current = true;
    if (streamIdentity && rawSessionID) {
      recordSessionPersistenceInvalidation(
        silentReplayRecoveryDiagnostic(
          {
            backendScopeID: streamIdentity.backendScopeID,
            factorySessionID: streamIdentity.factorySessionID,
            streamGenerationID: streamIdentity.streamGenerationID,
          },
          rawSessionID,
        ),
      );
    }
    resetTimeline();
    void clearTimelineCheckpoint(window.indexedDB, streamIdentity);
  }, [rawSessionID, resetTimeline, streamIdentity]);

  const checkpointSyncIdentity = useMemo(() => {
    if (
      streamIdentity?.backendScopeID &&
      streamIdentity.factorySessionID &&
      streamIdentity.streamGenerationID
    ) {
      const logicalSessionKeyId =
        currentReplayCheckpoint?.syncIdentity?.logicalSessionKeyId?.trim() ||
        streamIdentity.logicalSessionKeyID?.trim();
      if (logicalSessionKeyId) {
        return {
          backendScopeId: streamIdentity.backendScopeID,
          factorySessionId: streamIdentity.factorySessionID,
          logicalSessionKeyId,
          streamGenerationId: streamIdentity.streamGenerationID,
        };
      }
    }
    return currentReplayCheckpoint?.syncIdentity;
  }, [currentReplayCheckpoint?.syncIdentity, streamIdentity]);

  usePersistedTimelineCheckpoint({
    checkpoint: currentReplayCheckpoint,
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    streamIdentity,
    syncIdentity: checkpointSyncIdentity,
  });

  const handleStreamEvent = useCallback((event: FactoryEvent) => {
    queuedAppendRef.current([event]);
  }, []);

  const handleStreamEvents = useCallback((events: FactoryEvent[]) => {
    queuedAppendRef.current(events);
  }, []);

  useFactoryEventStream({
    enabled:
      checkpointHydrated &&
      preflightReady &&
      preflightRecovery == null &&
      effectiveSessionID != null &&
      !isPaused,
    initialReconnectCursor,
    locale,
    onEvent: handleStreamEvent,
    onEvents: handleStreamEvents,
    onInvalidReconnectCursor: handleInvalidReconnectCursor,
    refreshToken,
    sessionID: effectiveSessionID,
    streamIdentity,
  });

  useDashboardTimelineMemoryDebug({ debugOptions, eventCount });

  const preflightStatus: DashboardPreflightStatus =
    preflightRecovery != null
      ? "non-recoverable"
      : preflightError != null
        ? "non-recoverable"
        : checkpointHydrated && preflightReady
          ? "success"
          : "loading";

  return useMemo(
    () => ({
      error: preflightRecovery != null ? null : (preflightError ?? error),
      isInitialLoading:
        preflightRecovery == null &&
        preflightError == null &&
        isInitialLoading,
      preflightRecovery,
      preflightStatus,
      snapshot: preflightRecovery ? null : snapshot,
      streamState,
    }),
    [
      error,
      isInitialLoading,
      preflightError,
      preflightRecovery,
      preflightStatus,
      snapshot,
      streamState,
    ],
  );
}
