import type { components } from "../../../api/generated/openapi";

export type WorkerSessionEventRecord =
  components["schemas"]["WorkerSessionEventRecord"];

export type WorkerTimelineJSONObject = Record<string, unknown>;

export type WorkerTimelineJSONValue =
  | string
  | number
  | boolean
  | null
  | WorkerTimelineJSONValue[]
  | { [key: string]: WorkerTimelineJSONValue };

export type WorkerTimelineEntryCategory =
  | "session"
  | "run"
  | "turn"
  | "message"
  | "reasoning"
  | "tool"
  | "file-change"
  | "plan"
  | "progress"
  | "usage"
  | "error"
  | "stream-gap"
  | "generic";

export type WorkerTimelineTerminalOutcome = "SUCCESS" | "FAILURE" | "CANCELED";

export interface WorkerTimelineCanonicalSource {
  position: number;
  cursor: {
    position: number;
    workerSessionId?: string;
    streamGenerationId?: string;
  };
  sourceType: string;
  sourceId: string;
  sourceSequence: number;
  sourceEventId: string;
  schemaId: string;
}

export interface WorkerTimelineIdentity {
  provider?: string;
  modelProvider?: string;
  executorProvider?: string;
  model?: string;
}

export interface WorkerTimelineAttempt {
  id?: string;
  number?: number;
  reason?: string;
  dispatchId?: string;
  turnId?: string;
  status?: string;
}

export interface WorkerTimelineProviderSession {
  provider: string;
  kind: string;
  id: string;
}

export interface WorkerTimelineContinuation {
  providerSession?: WorkerTimelineProviderSession;
  predecessorWorkerSessionId?: string;
  successorWorkerSessionId?: string;
  previousDispatchId?: string;
  previousAttemptId?: string;
}

export interface WorkerTimelineContentBlock {
  kind: string;
  text?: string;
  toolCallId?: string;
  toolName?: string;
  argumentsSummary?: WorkerTimelineJSONValue;
  imageRef?: string;
  resourceRef?: string;
  structuredOutput?: WorkerTimelineJSONValue;
}

export interface WorkerTimelineMessageDelta {
  contentBlockIndex?: number;
  contentBlockKind?: string;
  textDelta?: string;
}

export interface WorkerTimelineMessage {
  role?: string;
  contentBlocks?: WorkerTimelineContentBlock[];
  text?: string;
  partial?: boolean;
  delta?: WorkerTimelineMessageDelta;
}

export interface WorkerTimelineReasoning {
  representation?: "SNAPSHOT" | "DELTA";
  summary?: string;
  summaryDelta?: string;
}

export interface WorkerTimelineProgress {
  label?: string;
  message?: string;
  percentComplete?: number;
}

export interface WorkerTimelineTool {
  toolCallId?: string;
  toolName?: string;
  status?: string;
  argumentsSummary?: WorkerTimelineJSONValue;
  resultSummary?: WorkerTimelineJSONValue;
  outputDelta?: string;
}

export interface WorkerTimelineUsage {
  inputTokens?: number;
  cachedInputTokens?: number;
  cacheWriteTokens?: number;
  outputTokens?: number;
  reasoningOutputTokens?: number;
  totalTokens?: number;
  model?: string;
}

export interface WorkerTimelineFailure {
  kind?: string;
  code?: string;
  message?: string;
  retryable?: boolean;
  retryAfterSeconds?: number;
  retryAttempt?: number;
}

export interface WorkerTimelineTerminal {
  outcome: WorkerTimelineTerminalOutcome;
  status?: string;
  failure?: WorkerTimelineFailure;
}

export interface WorkerTimelineGenericMetadata {
  schemaId: string;
  sourceType: string;
  sourceId: string;
  sourceSequence: number;
  payloadKeys: string[];
  payloadKeyCount: number;
  payloadKeysTruncated: boolean;
}

export interface WorkerSessionTimelineEntry {
  /** Stable React key composed from canonical position and source identity. */
  key: string;
  canonical: WorkerTimelineCanonicalSource;
  kind: string;
  phase: string;
  category: WorkerTimelineEntryCategory;
  itemId?: string;
  parentItemId?: string;
  identity?: WorkerTimelineIdentity;
  attempt?: WorkerTimelineAttempt;
  continuation?: WorkerTimelineContinuation;
  message?: WorkerTimelineMessage;
  reasoning?: WorkerTimelineReasoning;
  progress?: WorkerTimelineProgress;
  tool?: WorkerTimelineTool;
  usage?: WorkerTimelineUsage;
  failure?: WorkerTimelineFailure;
  terminal?: WorkerTimelineTerminal;
  generic?: WorkerTimelineGenericMetadata;
}
