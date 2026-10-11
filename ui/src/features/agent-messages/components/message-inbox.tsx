import { Text } from "@you-agent-factory/components/primitives";
import { useEffect, useId, useState } from "react";
import type {
  AgentMessage,
  MessageFilters,
} from "../../../api/agent-messages/types";
import { factoryAPIURL } from "../../../api/baseUrl";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { OptionalEnumSelect } from "../../../components/ui/enum-select";
import { FormField, FormLabel } from "../../../components/ui/form-field";
import { Input } from "../../../components/ui/input";
import {
  type MessageFeedOptions,
  useMessageFeed,
} from "../hooks/use-message-feed";
import { useMessageSuccessor } from "../hooks/use-message-successor";
import type { NavigateMessageWorker } from "../hooks/use-message-thread";
import type { MessageIdentityReader } from "../lib/read-message-scope";
import { getAgentMessageMessages } from "../messages/agent-messages";
import { MessageRecord } from "./message-record";

const statuses: AgentMessage["status"][] = [
  "QUEUED",
  "READ",
  "DELIVERED_REVIVE",
  "REPLIED",
  "EXPIRED",
];
const blank = { recipient: "", sender: "", thread: "", status: "" };

export function MessageInbox({
  locale,
  onNavigate,
  readIdentity,
  ...options
}: MessageFeedOptions & {
  locale?: string;
  onNavigate?: NavigateMessageWorker;
  readIdentity?: MessageIdentityReader;
}) {
  const copy = getAgentMessageMessages(locale);
  const id = useId();
  const [draft, setDraft] = useState(blank);
  const [filters, setFilters] = useState<MessageFilters>({});
  const [thread, setThread] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now);
  const state = useMessageFeed({
    ...options,
    filters: thread ? { threadId: thread } : filters,
  });
  const origin = options.origin ?? factoryAPIURL("").replace(/\/$/, "");
  const successor = useMessageSuccessor(
    origin,
    JSON.stringify([origin, filters, thread]),
    onNavigate,
    readIdentity,
  );
  useEffect(() => {
    const timer = globalThis.setInterval(() => setNow(Date.now()), 30_000);
    return () => globalThis.clearInterval(timer);
  }, []);
  return (
    <section
      aria-label={copy.inbox}
      aria-busy={state.busy}
      className="grid min-w-0 gap-3"
    >
      <form
        className="grid min-w-0 gap-3 sm:grid-cols-2"
        onSubmit={(event) => {
          event.preventDefault();
          setThread(null);
          setFilters({
            toWorkerSessionId: draft.recipient.trim() || undefined,
            fromWorkerSessionId: draft.sender.trim() || undefined,
            threadId: draft.thread.trim() || undefined,
            status: draft.status
              ? [draft.status as AgentMessage["status"]]
              : undefined,
          });
        }}
      >
        {(["recipient", "sender", "thread"] as const).map((field) => (
          <FormField key={field}>
            <FormLabel htmlFor={`${id}-${field}`}>
              {field === "recipient"
                ? copy.recipientFilter
                : field === "sender"
                  ? copy.senderFilter
                  : copy.threadFilter}
            </FormLabel>
            <Input
              id={`${id}-${field}`}
              value={draft[field]}
              onChange={(event) =>
                setDraft({ ...draft, [field]: event.target.value })
              }
            />
          </FormField>
        ))}
        <FormField>
          <FormLabel htmlFor={`${id}-status`}>{copy.statusFilter}</FormLabel>
          <OptionalEnumSelect
            id={`${id}-status`}
            value={draft.status || null}
            emptyOptionLabel={copy.allStatuses}
            onValueChange={(value) =>
              setDraft({ ...draft, status: value ?? "" })
            }
            options={statuses.map((status) => ({
              value: status,
              label: copy.status[status],
            }))}
          />
        </FormField>
        <DashboardActionButton type="submit">
          {copy.apply}
        </DashboardActionButton>
      </form>
      <div className="flex flex-wrap gap-2">
        <DashboardActionButton
          type="button"
          disabled={state.busy}
          onClick={state.refresh}
        >
          {copy.refresh}
        </DashboardActionButton>
        {thread ? (
          <DashboardActionButton type="button" onClick={() => setThread(null)}>
            {copy.back}
          </DashboardActionButton>
        ) : null}
      </div>
      <Text role="status" aria-live="polite">
        {state.busy
          ? copy.loading
          : state.error
            ? copy.failures[state.error.code]
            : state.newData
              ? copy.newData
              : !state.messages.length
                ? copy.empty
                : state.live
                  ? copy.live
                  : copy.stale}
      </Text>
      {state.busy && state.messages.length ? <Text>{copy.stale}</Text> : null}
      {state.error ? (
        <DashboardActionButton type="button" onClick={state.refresh}>
          {copy.retry}
        </DashboardActionButton>
      ) : null}
      {successor.error ? (
        <AlertPanel role="alert" tone="danger">
          {copy.successorError}
        </AlertPanel>
      ) : null}
      <ol className="m-0 grid min-w-0 list-none gap-3 p-0">
        {state.messages.map((message) => (
          <MessageRecord
            key={message.messageId}
            message={message}
            now={now}
            locale={locale}
            onThread={() => setThread(message.threadId)}
            onSuccessor={
              successor.open
                ? () => {
                    void successor.open?.(message);
                  }
                : undefined
            }
          />
        ))}
      </ol>
      <div className="flex flex-wrap gap-2">
        <DashboardActionButton
          type="button"
          disabled={state.busy || !state.previousAvailable}
          onClick={state.previous}
        >
          {copy.previous}
        </DashboardActionButton>
        <DashboardActionButton
          type="button"
          disabled={state.busy || !state.nextAvailable}
          onClick={state.next}
        >
          {copy.next}
        </DashboardActionButton>
      </div>
    </section>
  );
}
