import { Label, Text } from "@you-agent-factory/components/primitives";
import type { AgentMessage } from "../../../api/agent-messages/types";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { confirmedMessageSuccessor } from "../lib/message-scope";
import {
  getAgentMessageMessages,
  messageRelativeTime,
} from "../messages/agent-messages";

export function MessageRecord({
  message,
  locale,
  now,
  onThread,
  onSuccessor,
}: {
  message: AgentMessage;
  locale?: string;
  now: number;
  onThread: () => void;
  onSuccessor?: () => void;
}) {
  const copy = getAgentMessageMessages(locale);
  const reason = message.reason ? copy.reasons[message.reason] : undefined;
  return (
    <li className="grid min-w-0 gap-3 rounded-lg border border-outline p-3">
      <DashboardStatusPill>{copy.status[message.status]}</DashboardStatusPill>
      <Text className="whitespace-pre-wrap [overflow-wrap:anywhere]">
        {message.body}
      </Text>
      <dl className="grid min-w-0 gap-2 sm:grid-cols-2">
        <MessageFact label={copy.sender} value={message.from.workerSessionId} />
        <MessageFact
          label={copy.recipient}
          value={message.to.workerSessionId}
        />
        {message.inReplyTo ? (
          <MessageFact label={copy.reply} value={message.inReplyTo} />
        ) : null}
        <MessageFact
          label={copy.age}
          value={
            <time dateTime={message.sentAt} title={message.sentAt}>
              {messageRelativeTime(message.sentAt, now, locale)}
            </time>
          }
        />
        <MessageFact
          label={copy.expires}
          value={
            <time dateTime={message.expiresAt} title={message.expiresAt}>
              {messageRelativeTime(message.expiresAt, now, locale)}
            </time>
          }
        />
        {reason ? <MessageFact label={copy.reason} value={reason} /> : null}
      </dl>
      {message.ifEnded === "HOLD" ? <Text>{copy.hold}</Text> : null}
      <div className="flex flex-wrap gap-2">
        <DashboardActionButton
          onClick={onThread}
          type="button"
          aria-label={`${copy.thread} ${message.threadId}`}
        >
          {copy.thread}
        </DashboardActionButton>
        {confirmedMessageSuccessor(message) && onSuccessor ? (
          <DashboardActionButton
            onClick={onSuccessor}
            type="button"
            aria-label={`${copy.successor} ${confirmedMessageSuccessor(message)}`}
          >
            {copy.successor}
          </DashboardActionButton>
        ) : null}
      </div>
    </li>
  );
}

function MessageFact({
  label,
  value,
}: {
  label: string;
  value: import("react").ReactNode;
}) {
  return (
    <div className="grid min-w-0 gap-1">
      <Label as="dt">{label}</Label>
      <Text as="dd" className="m-0 [overflow-wrap:anywhere]">
        {value}
      </Text>
    </div>
  );
}
