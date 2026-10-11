import type {
  AgentMessage,
  MessageWorkerIdentity,
} from "../../../api/agent-messages/types";

function sameMessageWorker(
  a: MessageWorkerIdentity,
  b: MessageWorkerIdentity,
): boolean {
  return (
    a.workerSessionId === b.workerSessionId &&
    (a.factorySessionId ?? null) === (b.factorySessionId ?? null)
  );
}

export function linkedMessageWorker(
  source: MessageWorkerIdentity,
  candidate: MessageWorkerIdentity,
): boolean {
  if (sameMessageWorker(source, candidate)) return true;
  return (
    (source.successorWorkerSessionId === candidate.workerSessionId &&
      candidate.predecessorWorkerSessionId === source.workerSessionId) ||
    (source.predecessorWorkerSessionId === candidate.workerSessionId &&
      candidate.successorWorkerSessionId === source.workerSessionId) ||
    (source.continuationHeadWorkerSessionId === candidate.workerSessionId &&
      candidate.continuationHeadWorkerSessionId === candidate.workerSessionId)
  );
}

export function messageTouchesScope(
  message: AgentMessage,
  identities: MessageWorkerIdentity[],
): boolean {
  return identities.some((identity) =>
    [message.from, message.to].some(
      (endpoint) =>
        endpoint.workerSessionId === identity.workerSessionId &&
        (endpoint.factorySessionId ?? null) ===
          (identity.factorySessionId ?? null),
    ),
  );
}

export function scopedMessagePage(
  messages: AgentMessage[],
  identities: MessageWorkerIdentity[],
): AgentMessage[] {
  const seen = new Set<string>();
  return messages.filter((message) => {
    if (
      seen.has(message.messageId) ||
      !messageTouchesScope(message, identities)
    )
      return false;
    seen.add(message.messageId);
    return true;
  });
}

export function confirmedMessageSuccessor(
  message: AgentMessage,
): string | null {
  const revived = message.revivedWorkerSessionId;
  const delivered = message.to.deliveredWorkerSessionId;
  if (
    !revived ||
    !delivered ||
    revived !== delivered ||
    !["DELIVERED_REVIVE", "READ", "REPLIED", "EXPIRED"].includes(message.status)
  )
    return null;
  return revived;
}
