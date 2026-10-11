import { Heading, Text } from "@you-agent-factory/components/primitives";
import { useEffect, useRef, useState } from "react";
import { MessageReadError } from "../../../api/agent-messages/types";
import { factoryAPIURL } from "../../../api/baseUrl";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import {
  type MessageThreadOptions,
  type MessageThreadState,
  type NavigateMessageWorker,
  useMessageThread,
} from "../hooks/use-message-thread";
import { readConfirmedMessageSuccessor } from "../lib/read-message-scope";
import { getAgentMessageMessages } from "../messages/agent-messages";
import { MessageRecord } from "./message-record";

export function MessageThread({
  locale,
  onNavigate,
  ...options
}: MessageThreadOptions & {
  locale?: string;
  onNavigate?: NavigateMessageWorker;
}) {
  const state = useMessageThread(options);
  const [navigationError, setNavigationError] = useState(false);
  const navigation = useRef<AbortController | null>(null);
  const origin = options.origin ?? factoryAPIURL("").replace(/\/$/, "");
  const scopeKey = JSON.stringify([
    origin,
    options.workerSessionID,
    options.factorySessionID,
  ]);
  const navigationScope = useRef(scopeKey);
  useEffect(() => {
    navigationScope.current = scopeKey;
    setNavigationError(false);
    return () => {
      navigation.current?.abort();
    };
  }, [scopeKey]);
  return (
    <MessageThreadContent
      locale={locale}
      state={state}
      navigationError={navigationError}
      onSuccessor={
        onNavigate
          ? async (message) => {
              navigation.current?.abort();
              const controller = new AbortController();
              navigation.current = controller;
              setNavigationError(false);
              try {
                const identity = await readConfirmedMessageSuccessor(
                  message,
                  { origin, signal: controller.signal },
                  options.readIdentity,
                );
                if (
                  !controller.signal.aborted &&
                  navigationScope.current === scopeKey
                )
                  onNavigate(identity);
              } catch {
                if (
                  !controller.signal.aborted &&
                  navigationScope.current === scopeKey
                )
                  setNavigationError(true);
              }
            }
          : undefined
      }
    />
  );
}

export function MessageThreadContent({
  state,
  locale,
  navigationError = false,
  onSuccessor,
}: {
  state: MessageThreadState;
  locale?: string;
  navigationError?: boolean;
  onSuccessor?: (
    message: import("../../../api/agent-messages/types").AgentMessage,
  ) => void;
}) {
  const copy = getAgentMessageMessages(locale);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = globalThis.setInterval(() => setNow(Date.now()), 30_000);
    return () => globalThis.clearInterval(timer);
  }, []);
  return (
    <section
      aria-label={copy.title}
      aria-busy={state.busy}
      className="grid min-w-0 gap-3"
    >
      <Heading level="section">{copy.title}</Heading>
      {state.unavailable ? (
        <Text role="status">{copy.unavailable}</Text>
      ) : (
        <>
          <div className="flex flex-wrap gap-2">
            <DashboardActionButton
              type="button"
              aria-pressed={!state.sent}
              onClick={() => state.setSent(false)}
            >
              {copy.received}
            </DashboardActionButton>
            <DashboardActionButton
              type="button"
              aria-pressed={state.sent}
              onClick={() => state.setSent(true)}
            >
              {copy.sent}
            </DashboardActionButton>
            <DashboardActionButton
              type="button"
              disabled={state.busy}
              onClick={state.refresh}
            >
              {copy.refresh}
            </DashboardActionButton>
            {state.threadId ? (
              <DashboardActionButton
                type="button"
                onClick={() => state.setThread(null)}
              >
                {copy.back}
              </DashboardActionButton>
            ) : null}
          </div>
          {state.identity ? (
            <Text className="[overflow-wrap:anywhere]">
              {state.identity.workerSessionId}
            </Text>
          ) : null}
          {state.identities.length > 1 ? (
            <div className="flex flex-wrap gap-2">
              {state.identities.map((identity, index) => (
                <DashboardActionButton
                  key={`${identity.factorySessionId}/${identity.workerSessionId}`}
                  type="button"
                  aria-pressed={state.identity === identity}
                  onClick={() => state.setMember(index)}
                >
                  {identity.workerSessionId}
                </DashboardActionButton>
              ))}
            </div>
          ) : null}
          <Text role="status" aria-live="polite">
            {state.busy
              ? copy.loading
              : state.error
                ? copy.failures[
                    state.error instanceof MessageReadError
                      ? state.error.code
                      : "error"
                  ]
                : state.messages.length === 0
                  ? copy.empty
                  : copy.stale}
          </Text>
          {state.error ? (
            <DashboardActionButton type="button" onClick={state.refresh}>
              {copy.retry}
            </DashboardActionButton>
          ) : null}
          {navigationError ? (
            <AlertPanel role="alert" tone="danger">
              {copy.successorError}
            </AlertPanel>
          ) : null}
          <ol className="m-0 grid min-w-0 list-none gap-3 p-0">
            {state.messages.map((message) => (
              <MessageRecord
                key={message.messageId}
                message={message}
                locale={locale}
                now={now}
                onThread={() => state.setThread(message.threadId)}
                onSuccessor={
                  onSuccessor ? () => onSuccessor(message) : undefined
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
        </>
      )}
    </section>
  );
}
