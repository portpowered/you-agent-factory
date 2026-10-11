import type {
  AgentMessage,
  AgentMessageObservation,
  MessageFilters,
} from "../../../api/agent-messages/types";

export function matchesMessageFilters(
  message: AgentMessage,
  filters: MessageFilters,
): boolean {
  return (
    (!filters.toWorkerSessionId ||
      message.to.workerSessionId === filters.toWorkerSessionId) &&
    (!filters.factorySessionId ||
      !filters.toWorkerSessionId ||
      message.to.factorySessionId === filters.factorySessionId) &&
    (!filters.fromWorkerSessionId ||
      message.from.workerSessionId === filters.fromWorkerSessionId) &&
    (!filters.threadId || message.threadId === filters.threadId) &&
    (!filters.status?.length || filters.status.includes(message.status))
  );
}

/** Preserve server send order, including equal timestamps; sequence orders transitions only. */
export function reconcileMessagePage(
  messages: AgentMessage[],
  observations: ReadonlyMap<string, AgentMessageObservation>,
  filters: MessageFilters,
): AgentMessage[] {
  const seen = new Set<string>();
  return messages.flatMap((message) => {
    if (seen.has(message.messageId)) return [];
    seen.add(message.messageId);
    const updated = observations.get(message.messageId)?.message;
    // An observation cannot change immutable send/thread associations.
    const current = updated
      ? {
          ...updated,
          sentAt: message.sentAt,
          threadId: message.threadId,
          inReplyTo: message.inReplyTo,
          from: message.from,
          to: {
            ...message.to,
            deliveredWorkerSessionId: updated.to.deliveredWorkerSessionId,
          },
        }
      : message;
    return matchesMessageFilters(current, filters) ? [current] : [];
  });
}

export function retainMessageObservation(
  observations: Map<string, AgentMessageObservation>,
  observation: AgentMessageObservation,
): boolean {
  const previous = observations.get(observation.message.messageId);
  if (previous && previous.sequence >= observation.sequence) return false;
  observations.set(observation.message.messageId, observation);
  return true;
}
