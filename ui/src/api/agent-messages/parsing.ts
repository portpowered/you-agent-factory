import {
  AgentMessageDelivery,
  AgentMessageDeliveryStatus,
  AgentMessageIfEnded,
  AgentMessageObservationKind,
  AgentMessageReplyIfEnded,
} from "../generated/openapi";
import { isAPIRecord } from "../transport";
import {
  type AgentMessage,
  type AgentMessageObservation,
  type AgentMessagePage,
  MessageReadError,
  type MessageWorkerIdentity,
} from "./types";

function text(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function optionalText(value: unknown): boolean {
  return value == null || text(value);
}

function timestamp(value: unknown): boolean {
  return text(value) && Number.isFinite(Date.parse(value));
}

function integer(value: unknown): boolean {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function member(value: unknown, values: object): boolean {
  return Object.values(values).includes(value);
}

export function parseAgentMessage(value: unknown): AgentMessage {
  if (
    !isAPIRecord(value) ||
    !isAPIRecord(value.from) ||
    !isAPIRecord(value.to)
  ) {
    throw new MessageReadError("invalid");
  }
  const valid =
    text(value.messageId) &&
    text(value.threadId) &&
    optionalText(value.inReplyTo) &&
    value.from.principal === "WORKER" &&
    text(value.from.workerSessionId) &&
    optionalText(value.from.factorySessionId) &&
    value.to.kind === "WORKER_SESSION" &&
    text(value.to.workerSessionId) &&
    optionalText(value.to.factorySessionId) &&
    optionalText(value.to.deliveredWorkerSessionId) &&
    typeof value.body === "string" &&
    text(value.bodySha256) &&
    integer(value.bodyRedactionCount) &&
    member(value.delivery, AgentMessageDelivery) &&
    member(value.ifEnded, AgentMessageIfEnded) &&
    member(value.replyIfEnded, AgentMessageReplyIfEnded) &&
    member(value.status, AgentMessageDeliveryStatus) &&
    optionalText(value.reason) &&
    integer(value.hop) &&
    timestamp(value.sentAt) &&
    timestamp(value.expiresAt) &&
    optionalText(value.revivedWorkerSessionId) &&
    optionalText(value.repliedByMessageId);
  if (!valid) throw new MessageReadError("invalid");
  // The generated native record stays authoritative. Render only named public fields.
  return value as AgentMessage;
}

export function parseAgentMessagePage(value: unknown): AgentMessagePage {
  if (
    !isAPIRecord(value) ||
    !Array.isArray(value.messages) ||
    !optionalText(value.nextToken)
  ) {
    throw new MessageReadError("invalid");
  }
  return {
    messages: value.messages.map(parseAgentMessage),
    nextToken: value.nextToken as string | null | undefined,
  };
}

export function parseAgentMessageObservation(
  value: unknown,
): AgentMessageObservation {
  if (
    !isAPIRecord(value) ||
    !text(value.recordId) ||
    !integer(value.sequence) ||
    !member(value.kind, AgentMessageObservationKind)
  )
    throw new MessageReadError("invalid");
  return {
    recordId: value.recordId,
    sequence: value.sequence as number,
    kind: value.kind as AgentMessageObservation["kind"],
    message: parseAgentMessage(value.message),
  };
}

export function parseMessageWorkerIdentity(
  value: unknown,
): MessageWorkerIdentity {
  if (
    !isAPIRecord(value) ||
    !text(value.workerSessionId) ||
    !optionalText(value.factorySessionId) ||
    !optionalText(value.predecessorWorkerSessionId) ||
    !optionalText(value.successorWorkerSessionId) ||
    !optionalText(value.continuationHeadWorkerSessionId) ||
    (value.direct !== undefined && typeof value.direct !== "boolean")
  ) {
    throw new MessageReadError("invalid");
  }
  return {
    workerSessionId: value.workerSessionId,
    ...(typeof value.direct === "boolean" ? { direct: value.direct } : {}),
    ...Object.fromEntries(
      [
        "factorySessionId",
        "predecessorWorkerSessionId",
        "successorWorkerSessionId",
        "continuationHeadWorkerSessionId",
      ].flatMap((key) => (text(value[key]) ? [[key, value[key]]] : [])),
    ),
  };
}
