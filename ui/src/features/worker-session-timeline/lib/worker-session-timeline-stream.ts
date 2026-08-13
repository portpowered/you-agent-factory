import type {
  WorkerSessionEventCursor,
  WorkerSessionEventFrame,
  WorkerSessionEventReconnectCursor,
  WorkerSessionEventRecordingHealth,
  WorkerSessionReplaySummary,
} from "../../../api/worker-sessions";
import type { WorkerSessionEventRecord } from "./worker-session-timeline-projection-types";

export type WorkerSessionTimelineStreamStatus =
  | "idle"
  | "loading"
  | "ready-empty"
  | "live"
  | "reconnecting"
  | "completed"
  | "source-error";

export type WorkerSessionTimelineStreamErrorCode =
  | "STREAM_ERROR"
  | "SOURCE_FAILURE"
  | "INVALID_FRAME"
  | "SESSION_MISMATCH"
  | "CURSOR_FOREIGN"
  | "CURSOR_GENERATION_MISMATCH"
  | "CURSOR_INVALID"
  | "CURSOR_GAP"
  | "CURSOR_OUT_OF_ORDER"
  | "CURSOR_POSITION_CONFLICT";

export interface WorkerSessionTimelineStreamError {
  code: WorkerSessionTimelineStreamErrorCode;
  message: string;
}

export interface WorkerSessionTimelineStreamState {
  status: WorkerSessionTimelineStreamStatus;
  records: WorkerSessionEventRecord[];
  acknowledgedCursor: WorkerSessionEventCursor | null;
  streamGenerationId: string | null;
  recordingHealth: WorkerSessionEventRecordingHealth | null;
  recordingHealthReason: string | null;
  replaySummary: WorkerSessionReplaySummary | null;
  terminalDelivery: "TERMINAL" | "TERMINAL_REPLAY" | null;
  sourceError: WorkerSessionTimelineStreamError | null;
}

export interface WorkerSessionTimelineStreamContext {
  workerSessionID: string;
  initialReconnectCursor?: WorkerSessionEventReconnectCursor | null;
}

export interface WorkerSessionTimelineStreamTransition {
  state: WorkerSessionTimelineStreamState;
  shouldStop: boolean;
}

export interface WorkerSessionTimelineTransportFailure {
  state: WorkerSessionTimelineStreamState;
  shouldReconnect: boolean;
}

export function createWorkerSessionTimelineStreamState(
  initialReconnectCursor?: WorkerSessionEventReconnectCursor | null,
): WorkerSessionTimelineStreamState {
  return {
    status: "loading",
    records: [],
    acknowledgedCursor:
      initialReconnectCursor == null
        ? null
        : {
            position: initialReconnectCursor.afterPosition,
            ...(initialReconnectCursor.streamGenerationId
              ? {
                  streamGenerationId: initialReconnectCursor.streamGenerationId,
                }
              : {}),
          },
    streamGenerationId: initialReconnectCursor?.streamGenerationId ?? null,
    recordingHealth: null,
    recordingHealthReason: null,
    replaySummary: null,
    terminalDelivery: null,
    sourceError: null,
  };
}

export function workerSessionTimelineStreamCanFollow(
  state: WorkerSessionTimelineStreamState,
): boolean {
  return (
    state.status !== "idle" &&
    state.status !== "completed" &&
    state.status !== "source-error" &&
    state.replaySummary === null &&
    state.terminalDelivery === null
  );
}

export function workerSessionTimelineReconnectCursor(
  state: WorkerSessionTimelineStreamState,
): WorkerSessionEventReconnectCursor | null {
  if (state.acknowledgedCursor === null) {
    return null;
  }
  return {
    afterPosition: state.acknowledgedCursor.position,
    ...(state.streamGenerationId
      ? { streamGenerationId: state.streamGenerationId }
      : {}),
  };
}

export function markWorkerSessionTimelineStreamLive(
  state: WorkerSessionTimelineStreamState,
): WorkerSessionTimelineStreamState {
  if (!workerSessionTimelineStreamCanFollow(state)) {
    return state;
  }
  return {
    ...state,
    status: "live",
    sourceError: null,
  };
}

export function markWorkerSessionTimelineStreamReconnecting(
  state: WorkerSessionTimelineStreamState,
): WorkerSessionTimelineStreamState {
  if (!workerSessionTimelineStreamCanFollow(state)) {
    return state;
  }
  return {
    ...state,
    status: "reconnecting",
    sourceError: null,
  };
}

export function applyWorkerSessionTimelineFrame(
  state: WorkerSessionTimelineStreamState,
  frame: WorkerSessionEventFrame,
  context: WorkerSessionTimelineStreamContext,
): WorkerSessionTimelineStreamTransition {
  if (!workerSessionTimelineStreamCanFollow(state)) {
    return { state, shouldStop: true };
  }

  if (frame.workerSessionId !== context.workerSessionID) {
    return streamFailure(
      state,
      "SESSION_MISMATCH",
      "Worker Session event stream returned a different Worker Session.",
    );
  }

  const stateWithHealth = applyFrameHealth(state, frame);
  switch (frame.delivery) {
    case "RECORD":
      return applyRecordFrame(stateWithHealth, frame, context, null);
    case "TERMINAL":
    case "TERMINAL_REPLAY":
      return applyRecordFrame(stateWithHealth, frame, context, frame.delivery);
    case "REPLAY_SUMMARY":
      return applyReplaySummaryFrame(stateWithHealth, frame);
    case "SOURCE_FAILURE":
      return streamFailure(
        stateWithHealth,
        "SOURCE_FAILURE",
        frame.errorMessage ?? "Worker Session event source failed.",
      );
    default:
      return streamFailure(
        stateWithHealth,
        "INVALID_FRAME",
        "Worker Session event stream returned an unsupported delivery.",
      );
  }
}

export function applyWorkerSessionTimelineTransportFailure(
  state: WorkerSessionTimelineStreamState,
  message = "Worker Session event stream disconnected.",
): WorkerSessionTimelineTransportFailure {
  if (!workerSessionTimelineStreamCanFollow(state)) {
    return { state, shouldReconnect: false };
  }

  if (state.acknowledgedCursor !== null) {
    return {
      state: {
        ...state,
        status: "reconnecting",
        sourceError: null,
      },
      shouldReconnect: true,
    };
  }

  return {
    state: {
      ...state,
      status: "source-error",
      sourceError: { code: "STREAM_ERROR", message },
    },
    shouldReconnect: false,
  };
}

export function applyWorkerSessionTimelineParseFailure(
  state: WorkerSessionTimelineStreamState,
  message = "Worker Session event stream returned an invalid frame.",
): WorkerSessionTimelineStreamState {
  if (!workerSessionTimelineStreamCanFollow(state)) {
    return state;
  }
  return {
    ...state,
    status: "source-error",
    sourceError: { code: "INVALID_FRAME", message },
  };
}

function applyFrameHealth(
  state: WorkerSessionTimelineStreamState,
  frame: WorkerSessionEventFrame,
): WorkerSessionTimelineStreamState {
  return {
    ...state,
    ...(frame.recordingHealth !== undefined
      ? { recordingHealth: frame.recordingHealth }
      : {}),
    ...(frame.recordingHealthReason !== undefined
      ? { recordingHealthReason: frame.recordingHealthReason }
      : {}),
  };
}

function applyRecordFrame(
  state: WorkerSessionTimelineStreamState,
  frame: WorkerSessionEventFrame,
  context: WorkerSessionTimelineStreamContext,
  terminalDelivery: "TERMINAL" | "TERMINAL_REPLAY" | null,
): WorkerSessionTimelineStreamTransition {
  if (frame.event === null) {
    return streamFailure(
      state,
      "INVALID_FRAME",
      "Worker Session event stream returned a delivery without a record.",
    );
  }

  const cursorError = validateRecordCursor(state, frame.event, context);
  if (cursorError !== null) {
    return streamFailure(state, cursorError.code, cursorError.message);
  }

  const key = workerSessionEventRecordKey(frame.event);
  const duplicate = state.records.some(
    (record) => workerSessionEventRecordKey(record) === key,
  );
  if (duplicate) {
    return terminalDelivery !== null
      ? completeWithTerminal(state, terminalDelivery)
      : { state: { ...state, status: "live" }, shouldStop: false };
  }

  const positionError = validateRecordPosition(state, frame.event);
  if (positionError !== null) {
    return streamFailure(state, positionError.code, positionError.message);
  }

  const nextGenerationID =
    state.streamGenerationId ?? frame.event.cursor.streamGenerationId ?? null;
  const nextCursor = normalizedAcknowledgedCursor(
    frame.event.cursor,
    context.workerSessionID,
    nextGenerationID,
  );
  const nextState: WorkerSessionTimelineStreamState = {
    ...state,
    status: terminalDelivery !== null ? "completed" : "live",
    records: [...state.records, frame.event],
    acknowledgedCursor: nextCursor,
    streamGenerationId: nextGenerationID,
    sourceError: null,
    ...(terminalDelivery !== null ? { terminalDelivery } : {}),
  };
  return { state: nextState, shouldStop: terminalDelivery !== null };
}

function applyReplaySummaryFrame(
  state: WorkerSessionTimelineStreamState,
  frame: WorkerSessionEventFrame,
): WorkerSessionTimelineStreamTransition {
  if (frame.replaySummary === undefined || frame.replaySummary === null) {
    return streamFailure(
      state,
      "INVALID_FRAME",
      "Worker Session replay summary is unavailable.",
    );
  }
  return {
    state: {
      ...state,
      status:
        frame.replaySummary.eventsEmitted === 0 && state.records.length === 0
          ? "ready-empty"
          : "completed",
      replaySummary: frame.replaySummary,
      sourceError: null,
    },
    shouldStop: true,
  };
}

function validateRecordCursor(
  state: WorkerSessionTimelineStreamState,
  record: WorkerSessionEventRecord,
  context: WorkerSessionTimelineStreamContext,
): WorkerSessionTimelineStreamError | null {
  if (
    record.position !== record.cursor.position ||
    !Number.isInteger(record.position) ||
    record.position < 1
  ) {
    return {
      code: "CURSOR_INVALID",
      message: "Worker Session event cursor is invalid.",
    };
  }

  if (
    record.cursor.workerSessionId !== undefined &&
    record.cursor.workerSessionId !== context.workerSessionID
  ) {
    return {
      code: "CURSOR_FOREIGN",
      message: "Worker Session event cursor belongs to another Worker Session.",
    };
  }

  const expectedGenerationID = state.streamGenerationId;
  if (
    expectedGenerationID !== null &&
    record.cursor.streamGenerationId !== expectedGenerationID
  ) {
    return {
      code: "CURSOR_GENERATION_MISMATCH",
      message:
        "Worker Session event cursor belongs to another stream generation.",
    };
  }
  return null;
}

function validateRecordPosition(
  state: WorkerSessionTimelineStreamState,
  record: WorkerSessionEventRecord,
): WorkerSessionTimelineStreamError | null {
  const lastPosition = state.acknowledgedCursor?.position;
  if (
    lastPosition === undefined ||
    (state.records.length === 0 && lastPosition === 0)
  ) {
    return null;
  }
  if (lastPosition === undefined) {
    return null;
  }

  if (record.position > lastPosition + 1) {
    return {
      code: "CURSOR_GAP",
      message: "Worker Session event stream contains a cursor gap.",
    };
  }
  if (record.position <= lastPosition) {
    return {
      code:
        record.position === lastPosition
          ? "CURSOR_POSITION_CONFLICT"
          : "CURSOR_OUT_OF_ORDER",
      message: "Worker Session event stream returned records out of order.",
    };
  }
  return null;
}

function normalizedAcknowledgedCursor(
  cursor: WorkerSessionEventCursor,
  workerSessionID: string,
  streamGenerationID: string | null,
): WorkerSessionEventCursor {
  return {
    position: cursor.position,
    workerSessionId: cursor.workerSessionId ?? workerSessionID,
    ...(streamGenerationID !== null
      ? { streamGenerationId: streamGenerationID }
      : {}),
  };
}

function completeWithTerminal(
  state: WorkerSessionTimelineStreamState,
  delivery: "TERMINAL" | "TERMINAL_REPLAY",
): WorkerSessionTimelineStreamTransition {
  return {
    state: {
      ...state,
      status: "completed",
      terminalDelivery: delivery,
      sourceError: null,
    },
    shouldStop: true,
  };
}

function streamFailure(
  state: WorkerSessionTimelineStreamState,
  code: WorkerSessionTimelineStreamErrorCode,
  message: string,
): WorkerSessionTimelineStreamTransition {
  return {
    state: {
      ...state,
      status: "source-error",
      sourceError: { code, message },
    },
    shouldStop: true,
  };
}

function workerSessionEventRecordKey(record: WorkerSessionEventRecord): string {
  return JSON.stringify([
    record.position,
    record.sourceType,
    record.sourceId,
    record.sourceSequence,
    record.sourceEventId,
  ]);
}

export { workerSessionEventRecordKey };
