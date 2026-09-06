import { factoryAPIURL } from "../baseUrl";
import type {
  components,
  WorkerSessionEventRecordingHealth,
} from "../generated/openapi";
import { factorySessionScopedPath } from "../session-routing";
import { isAPIRecord } from "../transport";
import type {
  WorkerSessionEventFrame,
  WorkerSessionEventSourceLike,
  WorkerSessionEventStreamStatus,
} from "./types";

export type {
  WorkerSessionEventCursor,
  WorkerSessionEventDelivery,
  WorkerSessionEventFrame,
  WorkerSessionEventRecord,
  WorkerSessionEventSourceLike,
  WorkerSessionEventStreamStatus,
  WorkerSessionProviderSessionRef,
  WorkerSessionReplaySummary,
} from "./types";

export const WORKER_SESSION_EVENTS_ENDPOINT = "/worker-sessions";

export interface WorkerSessionEventReconnectCursor {
  afterPosition: number;
  streamGenerationId?: string;
}

export interface BuildWorkerSessionEventStreamURLOptions {
  factorySessionID: string;
  workerSessionID: string;
  replayOnly?: boolean;
  reconnect?: WorkerSessionEventReconnectCursor | null;
}

export interface OpenWorkerSessionEventStreamOptions
  extends BuildWorkerSessionEventStreamURLOptions {
  onFrame: (frame: WorkerSessionEventFrame) => void;
  onError?: (error: WorkerSessionEventStreamParseError) => void;
  onStatusChange: (
    status: WorkerSessionEventStreamStatus,
    message: string,
  ) => void;
}

export type WorkerSessionEventStreamParseErrorCode =
  | "INVALID_JSON"
  | "INVALID_FRAME";

export class WorkerSessionEventStreamParseError extends Error {
  public readonly code: WorkerSessionEventStreamParseErrorCode;

  public constructor(
    code: WorkerSessionEventStreamParseErrorCode,
    message: string,
  ) {
    super(message);
    this.name = "WorkerSessionEventStreamParseError";
    this.code = code;
  }
}

type WorkerSessionEventSourceConstructor = new (
  url: string,
) => WorkerSessionEventSourceLike;

const WORKER_SESSION_EVENT_DELIVERIES = new Set([
  "RECORD",
  "TERMINAL",
  "TERMINAL_REPLAY",
  "REPLAY_SUMMARY",
  "SOURCE_FAILURE",
]);

const WORKER_SESSION_RECORDING_HEALTH = new Set([
  "COMPLETE",
  "DEGRADED",
  "INCOMPLETE",
]);

function workerSessionEventSource(): WorkerSessionEventSourceConstructor | null {
  const eventSource = globalThis.EventSource;
  if (typeof eventSource === "undefined") {
    return null;
  }
  return eventSource as unknown as WorkerSessionEventSourceConstructor;
}

export function buildWorkerSessionEventStreamURL({
  factorySessionID,
  workerSessionID,
  replayOnly = false,
  reconnect,
}: BuildWorkerSessionEventStreamURLOptions): string {
  const path = factorySessionScopedPath(
    `${WORKER_SESSION_EVENTS_ENDPOINT}/${encodeURIComponent(workerSessionID)}/events`,
    factorySessionID,
  );
  const params = new URLSearchParams();
  if (replayOnly) {
    params.set("replayOnly", "true");
  }
  if (reconnect != null) {
    params.set("after_position", String(reconnect.afterPosition));
    if (reconnect.streamGenerationId) {
      params.set("stream_generation_id", reconnect.streamGenerationId);
    }
  }
  const query = params.toString();
  return factoryAPIURL(query.length > 0 ? `${path}?${query}` : path);
}

export function openWorkerSessionEventStream({
  factorySessionID,
  workerSessionID,
  replayOnly,
  reconnect,
  onFrame,
  onError,
  onStatusChange,
}: OpenWorkerSessionEventStreamOptions): WorkerSessionEventSourceLike | null {
  const EventSourceImpl = workerSessionEventSource();
  if (EventSourceImpl === null) {
    onStatusChange(
      "offline",
      "Worker Session event stream is unavailable in this browser.",
    );
    return null;
  }

  const reconnecting = reconnect != null;
  const stream = new EventSourceImpl(
    buildWorkerSessionEventStreamURL({
      factorySessionID,
      workerSessionID,
      replayOnly,
      reconnect,
    }),
  );
  onStatusChange(
    reconnecting ? "reconnecting" : "connecting",
    reconnecting
      ? "Reconnecting to Worker Session events..."
      : "Connecting to Worker Session events...",
  );
  stream.onopen = () => {
    onStatusChange("live", "Worker Session event stream connected.");
  };
  stream.onerror = () => {
    onStatusChange("offline", "Worker Session event stream disconnected.");
  };
  stream.addEventListener("message", (event) => {
    try {
      onFrame(parseWorkerSessionEventFrame(readSSEData(event)));
    } catch (error) {
      const parseError =
        error instanceof WorkerSessionEventStreamParseError
          ? error
          : new WorkerSessionEventStreamParseError(
              "INVALID_FRAME",
              "Worker Session event stream returned an invalid frame.",
            );
      onError?.(parseError);
    }
  });
  return stream;
}

function readSSEData(event: Event): unknown {
  const candidate = event as Event & { data?: unknown };
  if (typeof candidate.data !== "string") {
    throw new WorkerSessionEventStreamParseError(
      "INVALID_FRAME",
      "Worker Session event stream returned a frame without data.",
    );
  }
  try {
    return JSON.parse(candidate.data) as unknown;
  } catch {
    throw new WorkerSessionEventStreamParseError(
      "INVALID_JSON",
      "Worker Session event stream returned invalid JSON.",
    );
  }
}

export function parseWorkerSessionEventFrame(
  value: unknown,
): WorkerSessionEventFrame {
  if (!isAPIRecord(value)) {
    throw invalidFrame();
  }

  const delivery = value.delivery;
  if (
    typeof delivery !== "string" ||
    !WORKER_SESSION_EVENT_DELIVERIES.has(delivery)
  ) {
    throw invalidFrame();
  }
  if (!nonEmptyString(value.workerSessionId)) {
    throw invalidFrame();
  }
  if (
    value.factorySessionId !== undefined &&
    !nonEmptyString(value.factorySessionId)
  ) {
    throw invalidFrame();
  }
  let providerSession: WorkerSessionEventFrame["providerSession"] = null;
  if (value.providerSession !== null && value.providerSession !== undefined) {
    if (!isProviderSessionRef(value.providerSession)) {
      throw invalidFrame();
    }
    providerSession = value.providerSession;
  }
  if (!Array.isArray(value.workIds) || !value.workIds.every(isString)) {
    throw invalidFrame();
  }
  if (
    !(value.errorCode === null || typeof value.errorCode === "string") ||
    !(value.errorMessage === null || typeof value.errorMessage === "string")
  ) {
    throw invalidFrame();
  }

  const event =
    value.event === null ? null : parseWorkerSessionEventRecord(value.event);
  const replaySummary = parseReplaySummary(value.replaySummary);
  const recordingHealth = parseRecordingHealth(value.recordingHealth);
  if (
    value.recordingHealthReason !== undefined &&
    value.recordingHealthReason !== null &&
    typeof value.recordingHealthReason !== "string"
  ) {
    throw invalidFrame();
  }
  if (requiresRecord(delivery) && event === null) {
    throw invalidFrame();
  }
  if (
    !requiresRecord(delivery) &&
    delivery === "SOURCE_FAILURE" &&
    event !== null
  ) {
    throw invalidFrame();
  }

  return {
    delivery: delivery as WorkerSessionEventFrame["delivery"],
    workerSessionId: value.workerSessionId,
    ...(value.factorySessionId !== undefined
      ? { factorySessionId: value.factorySessionId }
      : {}),
    providerSession,
    workIds: value.workIds,
    event,
    errorCode: value.errorCode,
    errorMessage: value.errorMessage,
    ...(replaySummary !== undefined ? { replaySummary } : {}),
    ...(recordingHealth !== undefined ? { recordingHealth } : {}),
    ...(value.recordingHealthReason !== undefined
      ? { recordingHealthReason: value.recordingHealthReason }
      : {}),
  };
}

function parseWorkerSessionEventRecord(
  value: unknown,
): components["schemas"]["WorkerSessionEventRecord"] {
  if (!isAPIRecord(value) || !isAPIRecord(value.cursor)) {
    throw invalidFrame();
  }
  if (
    !positiveInteger(value.position) ||
    !positiveInteger(value.cursor.position) ||
    value.position !== value.cursor.position ||
    !nonEmptyString(value.sourceType) ||
    !nonEmptyString(value.sourceId) ||
    !positiveInteger(value.sourceSequence) ||
    !nonEmptyString(value.sourceEventId) ||
    !nonEmptyString(value.schemaId) ||
    !isAPIRecord(value.payload)
  ) {
    throw invalidFrame();
  }
  if (
    value.cursor.workerSessionId !== undefined &&
    typeof value.cursor.workerSessionId !== "string"
  ) {
    throw invalidFrame();
  }
  if (
    value.cursor.streamGenerationId !== undefined &&
    typeof value.cursor.streamGenerationId !== "string"
  ) {
    throw invalidFrame();
  }
  return value as components["schemas"]["WorkerSessionEventRecord"];
}

function parseReplaySummary(
  value: unknown,
): components["schemas"]["WorkerSessionReplaySummary"] | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  if (
    !isAPIRecord(value) ||
    value.kind !== "replay-summary" ||
    typeof value.complete !== "boolean" ||
    typeof value.reason !== "string" ||
    !nonNegativeInteger(value.eventsEmitted)
  ) {
    throw invalidFrame();
  }
  return value as components["schemas"]["WorkerSessionReplaySummary"];
}

function parseRecordingHealth(
  value: unknown,
): WorkerSessionEventRecordingHealth | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (
    typeof value !== "string" ||
    !WORKER_SESSION_RECORDING_HEALTH.has(value)
  ) {
    throw invalidFrame();
  }
  return value as WorkerSessionEventRecordingHealth;
}

function isProviderSessionRef(
  value: unknown,
): value is components["schemas"]["WorkerSessionProviderSessionRef"] {
  return (
    isAPIRecord(value) &&
    typeof value.provider === "string" &&
    typeof value.kind === "string" &&
    typeof value.id === "string"
  );
}

function requiresRecord(delivery: string): boolean {
  return (
    delivery === "RECORD" ||
    delivery === "TERMINAL" ||
    delivery === "TERMINAL_REPLAY"
  );
}

function nonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value > 0;
}

function nonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0;
}

function invalidFrame(): WorkerSessionEventStreamParseError {
  return new WorkerSessionEventStreamParseError(
    "INVALID_FRAME",
    "Worker Session event stream returned an invalid frame.",
  );
}
