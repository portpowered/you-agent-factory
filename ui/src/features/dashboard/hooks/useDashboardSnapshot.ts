import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryEvent } from "../../../api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  type FactoryTimelineCheckpoint,
  useFactoryTimelineStore,
} from "../../timeline/state/factoryTimelineStore";
import { readFactoryTimelineDebugOptions } from "../../timeline/state/factoryTimelineDebug";
import {
  clearTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timeline/state/timelineCheckpointPersistence";
import { reconnectCursorFromCheckpoint } from "../../timeline/state/timelineCheckpointReconnect";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
import { usePersistedTimelineCheckpoint } from "./checkpoint-lifecycle/usePersistedTimelineCheckpoint";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardCheckpointPreflight } from "./preflight/use-dashboard-checkpoint-preflight";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

export interface DashboardSnapshotResult {
  error: Error | null;
  isInitialLoading: boolean;
  preflightRecovery: ReturnType<
    typeof useDashboardCheckpointPreflight
  >["preflightRecovery"];
  preflightStatus: DashboardPreflightStatus;
  snapshot: ReturnType<typeof useDashboardWorldView>["snapshot"] | null;
  streamState: ReturnType<typeof useDashboardWorldView>["streamState"];
  workOutcomeStreamIdentity?: TimelineCheckpointStreamIdentity | null;
}

type DashboardPreflightStatus = "loading" | "non-recoverable" | "success";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: snapshot composition keeps preflight, checkpoint hydration, and stream wiring in one hook.
export function useDashboardSnapshot({
  locale,
  refreshToken = 0,
}: UseDashboardSnapshotOptions = {}): DashboardSnapshotResult {
  const activateTimelineEntry = useFactoryTimelineStore(
    (state) => state.activateEntry,
  );
  const appendEvents = useFactoryTimelineStore((state) => state.appendEvents);
  const appendEventsForEntry = useFactoryTimelineStore(
    (state) => state.appendEventsForEntry,
  );
  const currentReplayCheckpoint = useFactoryTimelineStore(
    (state) => state.currentReplayCheckpoint,
  );
  const eventCount = useFactoryTimelineStore((state) => state.events.length);
  const restoreCheckpointForEntry = useFactoryTimelineStore(
    (state) => state.restoreCheckpointForEntry,
  );
  const resetTimelineEntry = useFactoryTimelineStore(
    (state) => state.resetEntry,
  );
  const setResolvedStreamIdentity = useDashboardStreamStore(
    (state) => state.setResolvedStreamIdentity,
  );
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
  const lastPersistedCheckpointRef = useRef<FactoryTimelineCheckpoint | null>(
    null,
  );
  const lastSessionIDRef = useRef<string | null>(null);
  useDashboardSessionLifecycle({
    locale,
    refreshToken,
    sessionID: rawSessionID,
  });

  const {
    checkpointHydrated,
    cursorFreeReplayCorrelationToken,
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
    restoreCheckpoint: restoreCheckpointForEntry,
  });

  useEffect(() => {
    setResolvedStreamIdentity(streamIdentity ?? null);
  }, [setResolvedStreamIdentity, streamIdentity]);

  useEffect(() => {
    if (streamIdentity) {
      activateTimelineEntry(streamIdentity);
    }
  }, [activateTimelineEntry, streamIdentity]);

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
    [
      persistedCheckpoint,
      preflightReconnectCursor,
      reconnectCursorRevision,
      refreshToken,
    ],
  );

  const handleInvalidReconnectCursor = useCallback(() => {
    reconnectCursorInvalidatedRef.current = true;
    if (streamIdentity) {
      resetTimelineEntry(streamIdentity);
    }
    void clearTimelineCheckpoint(window.indexedDB, streamIdentity);
  }, [resetTimelineEntry, streamIdentity]);

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

  const appendStreamEvents = useCallback(
    (events: FactoryEvent[]) => {
      if (streamIdentity) {
        appendEventsForEntry(streamIdentity, events);
        return;
      }
      if (debugOptions.disableTimelineCheckpoint) {
        appendEvents(events);
      }
    },
    [
      appendEvents,
      appendEventsForEntry,
      debugOptions.disableTimelineCheckpoint,
      streamIdentity,
    ],
  );

  const handleStreamEvent = useCallback(
    (event: FactoryEvent) => {
      appendStreamEvents([event]);
    },
    [appendStreamEvents],
  );

  useFactoryEventStream({
    enabled:
      checkpointHydrated &&
      preflightReady &&
      preflightRecovery == null &&
      effectiveSessionID != null &&
      !isPaused,
    initialReconnectCursor,
    initialCursorFreeReplayCorrelationToken: cursorFreeReplayCorrelationToken,
    locale,
    onEvent: handleStreamEvent,
    onEvents: appendStreamEvents,
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
        preflightRecovery == null && preflightError == null && isInitialLoading,
      preflightRecovery,
      preflightStatus,
      snapshot: preflightRecovery ? null : snapshot,
      streamState,
      workOutcomeStreamIdentity: streamIdentity,
    }),
    [
      error,
      isInitialLoading,
      preflightError,
      preflightRecovery,
      preflightStatus,
      snapshot,
      streamState,
      streamIdentity,
    ],
  );
}
