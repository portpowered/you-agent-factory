import type { components } from "../generated/openapi";

export type AgentMessage = components["schemas"]["AgentMessage"];
export type AgentMessagePage =
  components["schemas"]["AgentMessageListResponse"];
export type AgentMessageObservation =
  components["schemas"]["AgentMessageObservation"];
export type MessageWorkerIdentity = Pick<
  components["schemas"]["WorkerSessionObservation"],
  | "workerSessionId"
  | "factorySessionId"
  | "predecessorWorkerSessionId"
  | "successorWorkerSessionId"
  | "continuationHeadWorkerSessionId"
> & { direct?: boolean };

/** Operator observation deliberately has no credentials or mutation options. */
export interface MessageReadOptions {
  origin: string;
  signal?: AbortSignal;
  fetch?: typeof globalThis.fetch;
}

export interface MessageFilters {
  factorySessionId?: string;
  toWorkerSessionId?: string;
  fromWorkerSessionId?: string;
  threadId?: string;
  status?: AgentMessage["status"][];
}

export interface MessageListOptions extends MessageReadOptions, MessageFilters {
  nextToken?: string;
}

export type MessageReadFailure =
  | "disabled"
  | "denied"
  | "unavailable"
  | "invalid"
  | "expired-page"
  | "gap"
  | "error";

export class MessageReadError extends Error {
  public constructor(public readonly code: MessageReadFailure) {
    super(code);
    this.name = "MessageReadError";
  }
}
