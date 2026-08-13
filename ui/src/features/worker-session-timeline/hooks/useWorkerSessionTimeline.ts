import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  type OpenWorkerSessionEventStreamOptions,
  openWorkerSessionEventStream,
  type WorkerSessionEventFrame,
  type WorkerSessionEventReconnectCursor,
  type WorkerSessionEventSourceLike,
  type WorkerSessionEventStreamStatus,
} from "../../../api/worker-sessions";
import {
  projectWorkerSessionTimeline,
  type WorkerSessionTimelineEntry,
} from "../lib/worker-session-timeline-projection";
import {
  applyWorkerSessionTimelineFrame,
  applyWorkerSessionTimelineParseFailure,
  applyWorkerSessionTimelineTransportFailure,
  createWorkerSessionTimelineStreamState,
  markWorkerSessionTimelineStreamLive,
  markWorkerSessionTimelineStreamReconnecting,
  type WorkerSessionTimelineStreamContext,
  type WorkerSessionTimelineStreamState,
  workerSessionTimelineReconnectCursor,
  workerSessionTimelineStreamCanFollow,
} from "../lib/worker-session-timeline-stream";

export type WorkerSessionTimelineOpenStream = (
  options: OpenWorkerSessionEventStreamOptions,
) => WorkerSessionEventSourceLike | null;

export type WorkerSessionTimelineReconnectScheduler = (
  callback: () => void,
  delayMs: number,
) => () => void;

export interface UseWorkerSessionTimelineOptions {
  factorySessionID: string | null;
  workerSessionID: string | null;
  enabled?: boolean;
  replayOnly?: boolean;
  initialReconnectCursor?: WorkerSessionEventReconnectCursor | null;
  openStream?: WorkerSessionTimelineOpenStream;
  reconnectDelayMs?: number;
  scheduleReconnect?: WorkerSessionTimelineReconnectScheduler;
}

export interface UseWorkerSessionTimelineResult
  extends WorkerSessionTimelineStreamState {
  entries: WorkerSessionTimelineEntry[];
  isFollowing: boolean;
  reconnectCursor: WorkerSessionEventReconnectCursor | null;
  retry: () => void;
}

const DEFAULT_RECONNECT_DELAY_MS = 1_000;

const defaultScheduleReconnect: WorkerSessionTimelineReconnectScheduler = (
  callback,
  delayMs,
) => {
  const handle = globalThis.setTimeout(callback, delayMs);
  return () => globalThis.clearTimeout(handle);
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stream lifecycle wiring stays with its single owning hook.
export function useWorkerSessionTimeline({
  factorySessionID,
  workerSessionID,
  enabled = true,
  replayOnly = false,
  initialReconnectCursor = null,
  openStream = openWorkerSessionEventStream,
  reconnectDelayMs = DEFAULT_RECONNECT_DELAY_MS,
  scheduleReconnect = defaultScheduleReconnect,
}: UseWorkerSessionTimelineOptions): UseWorkerSessionTimelineResult {
  const initialAfterPosition = initialReconnectCursor?.afterPosition;
  const initialGenerationID = initialReconnectCursor?.streamGenerationId;
  const stableInitialReconnectCursor = useMemo(
    () =>
      initialAfterPosition === undefined
        ? null
        : {
            afterPosition: initialAfterPosition,
            ...(initialGenerationID
              ? { streamGenerationId: initialGenerationID }
              : {}),
          },
    [initialAfterPosition, initialGenerationID],
  );
  const [state, setState] = useState<WorkerSessionTimelineStreamState>(() =>
    createWorkerSessionTimelineStreamState(stableInitialReconnectCursor),
  );
  const stateRef = useRef(state);
  const openStreamRef = useRef<WorkerSessionTimelineOpenStream>(openStream);
  const scheduleReconnectRef =
    useRef<WorkerSessionTimelineReconnectScheduler>(scheduleReconnect);
  const connectRef = useRef<(() => void) | null>(null);
  const cancelReconnectRef = useRef<(() => void) | null>(null);

  openStreamRef.current = openStream;
  scheduleReconnectRef.current = scheduleReconnect;
  stateRef.current = state;

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: one connection owns open, reconnect, terminal, and cleanup transitions.
  useEffect(() => {
    let disposed = false;
    const connectionRef: { current: WorkerSessionEventSourceLike | null } = {
      current: null,
    };
    const context: WorkerSessionTimelineStreamContext = {
      workerSessionID: workerSessionID ?? "",
      initialReconnectCursor: stableInitialReconnectCursor,
    };
    const targetIsReady =
      enabled && factorySessionID !== null && workerSessionID !== null;
    const attemptState = targetIsReady
      ? createWorkerSessionTimelineStreamState(stableInitialReconnectCursor)
      : {
          ...createWorkerSessionTimelineStreamState(null),
          status: "idle" as const,
        };
    stateRef.current = attemptState;
    setState(attemptState);
    cancelReconnectRef.current?.();
    cancelReconnectRef.current = null;

    const closeCurrentStream = () => {
      connectionRef.current?.close();
      connectionRef.current = null;
    };

    if (!targetIsReady) {
      connectRef.current = null;
      closeCurrentStream();
      return () => {
        disposed = true;
        closeCurrentStream();
      };
    }

    const updateState = (nextState: WorkerSessionTimelineStreamState) => {
      if (disposed) {
        return;
      }
      stateRef.current = nextState;
      setState(nextState);
    };

    const scheduleReconnectAttempt = () => {
      if (disposed || cancelReconnectRef.current !== null) {
        return;
      }
      const reconnectCursor = workerSessionTimelineReconnectCursor(
        stateRef.current,
      );
      if (reconnectCursor === null) {
        return;
      }
      const nextState = markWorkerSessionTimelineStreamReconnecting(
        stateRef.current,
      );
      updateState(nextState);
      cancelReconnectRef.current = scheduleReconnectRef.current(() => {
        cancelReconnectRef.current = null;
        connectRef.current?.();
      }, reconnectDelayMs);
    };

    const handleTransportStatus = (
      streamStatus: WorkerSessionEventStreamStatus,
      message: string,
    ) => {
      if (disposed) {
        return;
      }
      if (streamStatus === "live") {
        updateState(markWorkerSessionTimelineStreamLive(stateRef.current));
        return;
      }
      if (streamStatus === "reconnecting" || streamStatus === "connecting") {
        if (streamStatus === "reconnecting") {
          updateState(
            markWorkerSessionTimelineStreamReconnecting(stateRef.current),
          );
        }
        return;
      }

      closeCurrentStream();
      const failure = applyWorkerSessionTimelineTransportFailure(
        stateRef.current,
        message,
      );
      updateState(failure.state);
      if (failure.shouldReconnect) {
        scheduleReconnectAttempt();
      }
    };

    const handleFrame = (frame: WorkerSessionEventFrame) => {
      if (disposed) {
        return;
      }
      const transition = applyWorkerSessionTimelineFrame(
        stateRef.current,
        frame,
        context,
      );
      updateState(transition.state);
      if (transition.shouldStop) {
        cancelReconnectRef.current?.();
        cancelReconnectRef.current = null;
        closeCurrentStream();
      }
    };

    const handleParseError = (message: string) => {
      if (disposed) {
        return;
      }
      closeCurrentStream();
      updateState(
        applyWorkerSessionTimelineParseFailure(stateRef.current, message),
      );
    };

    const connect = () => {
      if (disposed || !workerSessionTimelineStreamCanFollow(stateRef.current)) {
        return;
      }
      closeCurrentStream();
      const reconnect = workerSessionTimelineReconnectCursor(stateRef.current);
      connectionRef.current = openStreamRef.current({
        factorySessionID,
        workerSessionID,
        replayOnly,
        reconnect,
        onFrame: handleFrame,
        onError: (error) => handleParseError(error.message),
        onStatusChange: handleTransportStatus,
      });
    };
    connectRef.current = connect;
    connect();

    return () => {
      disposed = true;
      connectRef.current = null;
      cancelReconnectRef.current?.();
      cancelReconnectRef.current = null;
      closeCurrentStream();
    };
  }, [
    enabled,
    factorySessionID,
    replayOnly,
    reconnectDelayMs,
    workerSessionID,
    stableInitialReconnectCursor,
  ]);

  const retry = useCallback(() => {
    const currentState = stateRef.current;
    if (
      currentState.terminalDelivery !== null ||
      currentState.replaySummary !== null ||
      factorySessionID === null ||
      workerSessionID === null
    ) {
      return;
    }
    const retryState: WorkerSessionTimelineStreamState = {
      ...currentState,
      status: currentState.records.length > 0 ? "reconnecting" : "loading",
      sourceError: null,
    };
    stateRef.current = retryState;
    setState(retryState);
    cancelReconnectRef.current?.();
    cancelReconnectRef.current = null;
    connectRef.current?.();
  }, [factorySessionID, workerSessionID]);

  const entries = useMemo(
    () => projectWorkerSessionTimeline(state.records),
    [state.records],
  );

  return {
    ...state,
    entries,
    isFollowing: state.status === "live" || state.status === "reconnecting",
    reconnectCursor: workerSessionTimelineReconnectCursor(state),
    retry,
  };
}
