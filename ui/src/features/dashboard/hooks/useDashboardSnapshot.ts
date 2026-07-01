import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { FactoryEvent } from "../../../api/events";
import { getFactorySession } from "../../../api/factory-sessions";
import {
  clearTimelineCheckpoint,
  type FactoryTimelineCheckpoint,
  persistTimelineCheckpoint,
  purgeLegacyTimelineCheckpoints,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
  useFactoryTimelineStore,
} from "../../timeline/public";
import { useDashboardSession } from "../session/dashboard-session-provider";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";
import { useFactoryEventStream } from "./event-stream/useFactoryEventStream";
import { useDashboardInitialReconnectCursor } from "./useDashboardInitialReconnectCursor";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";
import { useDashboardTimelineMemoryDebug } from "./useDashboardTimelineMemoryDebug";
import { useDashboardWorldView } from "./useDashboardWorldView";
import { readFactoryTimelineDebugOptions } from "../../timeline/state/factoryTimelineDebug";

export interface UseDashboardSnapshotOptions {
  locale?: string | null;
  refreshToken?: number;
}

interface DashboardSessionRecoveryState {
  reasonCode: string;
  requestedSessionId: string;
}

type DashboardPreflightStatus = "loading" | "non-recoverable" | "success";

function useGuardedTimelineCheckpointBootstrap({
  checkpointHydrationKey,
  checkpointsDisabled,
  rawSessionID,
  restoreCheckpoint,
  setResolvedStreamIdentity,
}: {
  checkpointHydrationKey: string | null;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  restoreCheckpoint: (checkpoint: FactoryTimelineCheckpoint) => void;
  setResolvedStreamIdentity: (
    streamIdentity: TimelineCheckpointStreamIdentity | null,
  ) => void;
}) {
  const setStreamState = useDashboardStreamStore((state) => state.setStreamState);
  const [checkpointHydratedKey, setCheckpointHydratedKey] =
    useState<string | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [preflightReadyKey, setPreflightReadyKey] = useState<string | null>(
    null,
  );
  const [preflightError, setPreflightError] = useState<Error | null>(null);
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);

  const checkpointHydrated = checkpointHydratedKey === checkpointHydrationKey;
  const preflightReady = preflightReadyKey === checkpointHydrationKey;

  useEffect(() => {
    let cancelled = false;

    setCheckpointHydratedKey(null);
    setPersistedCheckpoint(null);
    setPreflightError(null);
    setPreflightReadyKey(null);
    setStreamIdentity(null);
    setResolvedStreamIdentity(null);

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

    void getFactorySession(rawSessionID)
      .then(async (response) => {
        if (cancelled) {
          return;
        }
        const checkpointStreamIdentity = streamIdentityFromSessionResponse(
          response.session,
        );
        setStreamIdentity(checkpointStreamIdentity);
        setResolvedStreamIdentity(checkpointStreamIdentity);
        await purgeLegacyTimelineCheckpoints(window.indexedDB);
        setPreflightReadyKey(checkpointHydrationKey);
        const checkpoint = await readTimelineCheckpoint(
          window.indexedDB,
          checkpointStreamIdentity,
        );
        if (cancelled) {
          return;
        }
        if (checkpoint) {
          restoreCheckpoint(checkpoint);
        }
        setPersistedCheckpoint(checkpoint);
        setCheckpointHydratedKey(checkpointHydrationKey);
      })
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
    restoreCheckpoint,
    setResolvedStreamIdentity,
    setStreamState,
  ]);

  return {
    checkpointHydrated,
    preflightError,
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  };
}

function usePersistedTimelineCheckpoint({
  checkpoint,
  checkpointsDisabled,
  streamIdentity,
}: {
  checkpoint: FactoryTimelineCheckpoint | undefined;
  checkpointsDisabled: boolean;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}) {
  useEffect(() => {
    if (typeof window === "undefined" || checkpointsDisabled) {
      return;
    }
    const persistHandle = window.setTimeout(() => {
      void persistTimelineCheckpoint(
        window.indexedDB,
        checkpoint,
        streamIdentity,
      );
    }, 750);
    return () => {
      window.clearTimeout(persistHandle);
    };
  }, [checkpoint, checkpointsDisabled, streamIdentity]);
}

function streamIdentityFromSessionResponse(session: {
  runtime?: {
    streamIdentity?: {
      backendScopeID?: string;
      factorySessionID?: string;
      logicalSessionKeyID?: string;
      streamGenerationID?: string;
    };
  };
}): TimelineCheckpointStreamIdentity | null {
  const identity = session.runtime?.streamIdentity;
  if (
    typeof identity?.backendScopeID !== "string" ||
    typeof identity.factorySessionID !== "string" ||
    typeof identity.logicalSessionKeyID !== "string" ||
    typeof identity.streamGenerationID !== "string"
  ) {
    return null;
  }
  return {
    backendScopeID: identity.backendScopeID,
    factorySessionID: identity.factorySessionID,
    logicalSessionKeyID: identity.logicalSessionKeyID,
    streamGenerationID: identity.streamGenerationID,
  };
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
  const invalidatedReconnectCursorRef = useRef(false);
  const lastPersistedCheckpointRef =
    useRef<FactoryTimelineCheckpoint | null>(null);
  const lastSessionKeyRef = useRef<string | null>(null);
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
    preflightError,
    persistedCheckpoint,
    preflightReady,
    streamIdentity,
  } = useGuardedTimelineCheckpointBootstrap({
    checkpointHydrationKey,
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    rawSessionID,
    restoreCheckpoint,
    setResolvedStreamIdentity,
  });

  const resumedReconnectCursor = useDashboardInitialReconnectCursor({
    persistedCheckpoint,
    refreshToken,
    sessionID: rawSessionID,
  });

  if (
    lastPersistedCheckpointRef.current !== persistedCheckpoint ||
    lastSessionKeyRef.current !== checkpointHydrationKey
  ) {
    invalidatedReconnectCursorRef.current = false;
    lastPersistedCheckpointRef.current = persistedCheckpoint;
    lastSessionKeyRef.current = checkpointHydrationKey;
  }

  const initialReconnectCursor = useMemo(
    () =>
      invalidatedReconnectCursorRef.current
        ? undefined
        : resumedReconnectCursor,
    [resumedReconnectCursor],
  );

  const handleInvalidReconnectCursor = useCallback(() => {
    invalidatedReconnectCursorRef.current = true;
    resetTimeline();
    void clearTimelineCheckpoint(window.indexedDB, streamIdentity);
  }, [resetTimeline, streamIdentity]);

  usePersistedTimelineCheckpoint({
    checkpoint: currentReplayCheckpoint,
    checkpointsDisabled: debugOptions.disableTimelineCheckpoint,
    streamIdentity,
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
      rawSessionID != null &&
      !isPaused &&
      streamIdentity != null,
    initialReconnectCursor,
    locale,
    onEvent: handleStreamEvent,
    onEvents: handleStreamEvents,
    onInvalidReconnectCursor: handleInvalidReconnectCursor,
    refreshToken,
    sessionID: rawSessionID,
    streamIdentity,
  });

  useDashboardTimelineMemoryDebug({ debugOptions, eventCount });

  const preflightStatus: DashboardPreflightStatus =
    preflightError != null
      ? "non-recoverable"
      : checkpointHydrated && preflightReady
        ? "success"
        : "loading";

  return useMemo(
    () => ({
      error: preflightError ?? error,
      isInitialLoading: preflightError == null && isInitialLoading,
      preflightRecovery: null as DashboardSessionRecoveryState | null,
      preflightStatus,
      snapshot,
      streamState,
    }),
    [
      error,
      isInitialLoading,
      preflightError,
      preflightStatus,
      snapshot,
      streamState,
    ],
  );
}
