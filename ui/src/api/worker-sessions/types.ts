import type {
  components,
  WorkerSessionEventRecordingHealth as GeneratedWorkerSessionEventRecordingHealth,
} from "../generated/openapi";

export type WorkerSessionEventRecord =
  components["schemas"]["WorkerSessionEventRecord"];
export type WorkerSessionObservation =
  components["schemas"]["WorkerSessionObservation"];
export type WorkerSessionEventCursor =
  components["schemas"]["WorkerSessionEventCursor"];
export type WorkerSessionReplaySummary =
  components["schemas"]["WorkerSessionReplaySummary"];
export type WorkerSessionProviderSessionRef =
  components["schemas"]["WorkerSessionProviderSessionRef"];
export type WorkerSessionEventDelivery =
  components["schemas"]["WorkerSessionEventDelivery"];
export type WorkerSessionEventRecordingHealth =
  GeneratedWorkerSessionEventRecordingHealth;

/**
 * The generated OpenAPI type currently models `event` as required even though
 * the contract makes it nullable for REPLAY_SUMMARY and SOURCE_FAILURE frames.
 * Keep that transport correction at the handwritten API boundary.
 */
export interface WorkerSessionEventFrame {
  delivery: WorkerSessionEventDelivery;
  workerSessionId: string;
  factorySessionId?: string;
  providerSession: WorkerSessionProviderSessionRef | null;
  workIds: string[];
  event: WorkerSessionEventRecord | null;
  errorCode: string | null;
  errorMessage: string | null;
  replaySummary?: WorkerSessionReplaySummary | null;
  recordingHealth?: WorkerSessionEventRecordingHealth;
  recordingHealthReason?: string | null;
}

export interface WorkerSessionEventSourceLike {
  addEventListener: (type: string, listener: EventListener) => void;
  close: () => void;
  onerror: ((event: Event) => void) | null;
  onopen: ((event: Event) => void) | null;
}

export type WorkerSessionEventStreamStatus =
  | "connecting"
  | "live"
  | "reconnecting"
  | "offline";
