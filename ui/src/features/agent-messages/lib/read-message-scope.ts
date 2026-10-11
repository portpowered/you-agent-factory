import { readMessageWorkerIdentity } from "../../../api/agent-messages/api";
import {
  type AgentMessage,
  MessageReadError,
  type MessageReadOptions,
  type MessageWorkerIdentity,
} from "../../../api/agent-messages/types";
import {
  confirmedMessageSuccessor,
  linkedMessageWorker,
} from "./message-scope";

export type MessageIdentityReader = typeof readMessageWorkerIdentity;

export async function readMessageScope(
  options: MessageReadOptions & {
    workerSessionId: string;
    factorySessionId?: string;
  },
  reader: MessageIdentityReader = readMessageWorkerIdentity,
): Promise<MessageWorkerIdentity[]> {
  const source = await reader(options);
  if (
    source.workerSessionId !== options.workerSessionId ||
    (source.factorySessionId ?? null) !== (options.factorySessionId ?? null)
  ) {
    throw new MessageReadError("unavailable");
  }
  const ids = [
    ...new Set([
      source.predecessorWorkerSessionId,
      source.successorWorkerSessionId,
      source.continuationHeadWorkerSessionId,
    ]),
  ].filter((id): id is string => Boolean(id) && id !== source.workerSessionId);
  const related = await Promise.all(
    ids.map(async (workerSessionId) => {
      // Resolve the associated session's own scope. Never copy the source's scope.
      const candidate = await reader({
        origin: options.origin,
        signal: options.signal,
        workerSessionId,
      });
      if (
        candidate.workerSessionId !== workerSessionId ||
        !linkedMessageWorker(source, candidate)
      )
        throw new MessageReadError("unavailable");
      return candidate;
    }),
  );
  return [source, ...related];
}

export async function readConfirmedMessageSuccessor(
  message: AgentMessage,
  options: MessageReadOptions,
  reader: MessageIdentityReader = readMessageWorkerIdentity,
): Promise<MessageWorkerIdentity> {
  const successorId = confirmedMessageSuccessor(message);
  if (!successorId) throw new MessageReadError("unavailable");
  const recipient = await reader({
    ...options,
    workerSessionId: message.to.workerSessionId,
    factorySessionId: message.to.factorySessionId ?? undefined,
  });
  if (
    recipient.workerSessionId !== message.to.workerSessionId ||
    (recipient.factorySessionId ?? null) !==
      (message.to.factorySessionId ?? null)
  )
    throw new MessageReadError("unavailable");
  const successor = await reader({ ...options, workerSessionId: successorId });
  if (
    successor.workerSessionId !== successorId ||
    !linkedMessageWorker(recipient, successor)
  )
    throw new MessageReadError("unavailable");
  return successor;
}
