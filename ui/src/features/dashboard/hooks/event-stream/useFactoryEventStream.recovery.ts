import type { QueryClient } from "@tanstack/react-query";
import { type RefObject, useMemo, useRef } from "react";

import type {
  FactoryEventReconnectCursor,
  probeFactoryEventStreamRecovery,
} from "../../../../api/events";
import {
  deleteTimelineCheckpoint,
} from "../../../timeline/public/checkpoint-persistence";
import type { StreamDerivedCacheIdentity } from "../../../timeline/public/stream-identity";
import { recoverDashboardSessionScopedState } from "../../lib/dashboard-session-lifecycle";
import {
  correlationTokenForIdentityScope,
  recordSessionPersistenceDiagnostic,
  sessionPersistenceDiagnostic,
} from "../../lib/session-persistence/diagnostics";
import { getDashboardSessionLifecycleMessages } from "../../messages/dashboard-session-lifecycle";
import type { useDashboardStreamStore } from "../../state/dashboardStreamStore";

export interface DashboardStreamConnectionRefs {
  cursorFreeReplayPendingRef: RefObject<boolean>;
  cursorFreeReplayCorrelationTokenRef: RefObject<string | null>;
  hasOpenedStreamRef: RefObject<boolean>;
  lastSessionKeyRef: RefObject<string | null>;
  reconnectValidationAttemptRef: RefObject<number>;
  reconnectCursorRef: RefObject<FactoryEventReconnectCursor | undefined>;
  staleCursorRecoveryAttemptedRef: RefObject<boolean>;
  reconnectTimeoutRef: RefObject<number | null>;
  recoveringRef: RefObject<boolean>;
  streamRef: RefObject<{ close: () => void } | null>;
}

export function useDashboardStreamConnectionRefs(): DashboardStreamConnectionRefs {
  const cursorFreeReplayPendingRef = useRef(false);
  const cursorFreeReplayCorrelationTokenRef = useRef<string | null>(null);
  const hasOpenedStreamRef = useRef(false);
  const lastSessionKeyRef = useRef<string | null>(null);
  const reconnectValidationAttemptRef = useRef(0);
  const reconnectCursorRef = useRef<FactoryEventReconnectCursor | undefined>(
    undefined,
  );
  const staleCursorRecoveryAttemptedRef = useRef(false);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const recoveringRef = useRef(false);
  const streamRef = useRef<{ close: () => void } | null>(null);

  return useMemo(
    () => ({
      cursorFreeReplayPendingRef,
      cursorFreeReplayCorrelationTokenRef,
      hasOpenedStreamRef,
      lastSessionKeyRef,
      reconnectValidationAttemptRef,
      reconnectCursorRef,
      staleCursorRecoveryAttemptedRef,
      reconnectTimeoutRef,
      recoveringRef,
      streamRef,
    }),
    [],
  );
}

export function hasReconnectCursor(
  cursor: FactoryEventReconnectCursor | undefined,
): cursor is FactoryEventReconnectCursor {
  return Boolean(cursor?.afterEventId || cursor?.afterSequence != null);
}

export function recordStaleCursorDiagnostic(
  streamIdentity: StreamDerivedCacheIdentity | null,
  streamSessionID: string,
): string | null {
  try {
    const correlationToken = correlationTokenForIdentityScope(
      streamIdentity ?? { factorySessionID: streamSessionID },
    );
    return recordSessionPersistenceDiagnostic(
      sessionPersistenceDiagnostic("stale_cursor", correlationToken),
    )
      ? correlationToken
      : null;
  } catch {
    return null;
  }
}

export function recordCursorFreeReplayFallbackDiagnostic(
  correlationToken: string,
): void {
  try {
    recordSessionPersistenceDiagnostic(
      sessionPersistenceDiagnostic(
        "cursor_free_replay_fallback",
        correlationToken,
      ),
    );
  } catch {
    // Diagnostics are best effort and cannot affect stream recovery.
  }
}

export function reconnectingStreamState(locale?: string | null) {
  return {
    message:
      getDashboardSessionLifecycleMessages(locale).reconnectingStreamLabel,
    status: "reconnecting" as const,
  };
}

export function recoveryFailedStreamState(locale?: string | null) {
  return {
    message:
      getDashboardSessionLifecycleMessages(locale).recoveryFailedStreamLabel,
    status: "recovery_failed" as const,
  };
}

export function scheduleGenericReconnect({
  cursor,
  locale,
  openDashboardStream,
  reconnectTimeoutRef,
  setStreamState,
}: {
  cursor: FactoryEventReconnectCursor;
  locale?: string | null;
  openDashboardStream: (reconnect?: FactoryEventReconnectCursor) => void;
  reconnectTimeoutRef: RefObject<number | null>;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
}): void {
  setStreamState(reconnectingStreamState(locale));
  reconnectTimeoutRef.current = window.setTimeout(() => {
    reconnectTimeoutRef.current = null;
    openDashboardStream(cursor);
  }, 1000);
}

async function recoverStaleCursor({
  cursor,
  disposed,
  locale,
  onInvalidReconnectCursor,
  openDashboardStream,
  probeRecovery,
  cursorFreeReplayPendingRef,
  cursorFreeReplayCorrelationTokenRef,
  queryClient,
  queuedEventsRef,
  reconnectCursorRef,
  resetTimeline,
  setStreamState,
  staleCursorRecoveryAttemptedRef,
  streamIdentity,
  streamSessionID,
}: {
  cursor: FactoryEventReconnectCursor;
  disposed: RefObject<boolean>;
  locale?: string | null;
  onInvalidReconnectCursor?: () => void;
  openDashboardStream: (reconnect?: FactoryEventReconnectCursor) => void;
  probeRecovery: typeof probeFactoryEventStreamRecovery;
  cursorFreeReplayPendingRef: RefObject<boolean>;
  cursorFreeReplayCorrelationTokenRef: RefObject<string | null>;
  queryClient: QueryClient;
  queuedEventsRef: RefObject<unknown[]>;
  reconnectCursorRef: RefObject<FactoryEventReconnectCursor | undefined>;
  resetTimeline: () => void;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
  staleCursorRecoveryAttemptedRef: RefObject<boolean>;
  streamIdentity: StreamDerivedCacheIdentity | null;
  streamSessionID: string;
}): Promise<boolean> {
  try {
    const recovery = await probeRecovery(streamSessionID, cursor);
    if (
      disposed.current ||
      recovery.outcome !== "CURSOR_STALE" ||
      recovery.factorySessionId !== streamSessionID
    ) {
      return false;
    }

    staleCursorRecoveryAttemptedRef.current = true;
    cursorFreeReplayPendingRef.current = true;
    reconnectCursorRef.current = undefined;
    queuedEventsRef.current = [];
    cursorFreeReplayCorrelationTokenRef.current = recordStaleCursorDiagnostic(
      streamIdentity,
      streamSessionID,
    );
    onInvalidReconnectCursor?.();
    recoverDashboardSessionScopedState(
      queryClient,
      streamSessionID,
      resetTimeline,
      streamIdentity,
    );
    if (typeof window !== "undefined") {
      await deleteTimelineCheckpoint(window.indexedDB, streamIdentity);
    }
    setStreamState(reconnectingStreamState(locale));
    openDashboardStream(undefined);
    return true;
  } catch {
    return false;
  }
}

export async function reconnectAfterStreamError({
  cursor,
  disposed,
  locale,
  onInvalidReconnectCursor,
  openDashboardStream,
  probeRecovery,
  queryClient,
  queuedEventsRef,
  refs,
  resetTimeline,
  setStreamState,
  streamIdentity,
  streamSessionID,
}: {
  cursor: FactoryEventReconnectCursor;
  disposed: RefObject<boolean>;
  locale?: string | null;
  onInvalidReconnectCursor?: () => void;
  openDashboardStream: (reconnect?: FactoryEventReconnectCursor) => void;
  probeRecovery: typeof probeFactoryEventStreamRecovery;
  queryClient: QueryClient;
  queuedEventsRef: RefObject<unknown[]>;
  refs: DashboardStreamConnectionRefs;
  resetTimeline: () => void;
  setStreamState: (
    streamState: ReturnType<
      typeof useDashboardStreamStore.getState
    >["streamState"],
  ) => void;
  streamIdentity: StreamDerivedCacheIdentity | null;
  streamSessionID: string;
}): Promise<void> {
  if (!refs.staleCursorRecoveryAttemptedRef.current) {
    refs.recoveringRef.current = true;
    try {
      const recovered = await recoverStaleCursor({
        cursor,
        disposed,
        locale,
        onInvalidReconnectCursor,
        openDashboardStream,
        probeRecovery,
        cursorFreeReplayPendingRef: refs.cursorFreeReplayPendingRef,
        cursorFreeReplayCorrelationTokenRef:
          refs.cursorFreeReplayCorrelationTokenRef,
        queryClient,
        queuedEventsRef,
        reconnectCursorRef: refs.reconnectCursorRef,
        resetTimeline,
        setStreamState,
        staleCursorRecoveryAttemptedRef: refs.staleCursorRecoveryAttemptedRef,
        streamIdentity,
        streamSessionID,
      });
      if (recovered) {
        return;
      }
    } finally {
      refs.recoveringRef.current = false;
    }
  }

  if (disposed.current) {
    return;
  }
  scheduleGenericReconnect({
    cursor,
    locale,
    openDashboardStream,
    reconnectTimeoutRef: refs.reconnectTimeoutRef,
    setStreamState,
  });
}
