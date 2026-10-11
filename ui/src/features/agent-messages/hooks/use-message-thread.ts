import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { listAgentMessages } from "../../../api/agent-messages/api";
import type { MessageWorkerIdentity } from "../../../api/agent-messages/types";
import { factoryAPIURL } from "../../../api/baseUrl";
import { scopedMessagePage } from "../lib/message-scope";
import {
  type MessageIdentityReader,
  readMessageScope,
} from "../lib/read-message-scope";
import { type MessageFeedOptions, useMessageFeed } from "./use-message-feed";

export interface MessageThreadOptions {
  factorySessionID: string | null;
  workerSessionID: string | null;
  enabled?: boolean;
  origin?: string;
  readIdentity?: MessageIdentityReader;
  list?: typeof listAgentMessages;
  follow?: MessageFeedOptions["follow"];
}

export function useMessageThread({
  factorySessionID,
  workerSessionID,
  enabled = true,
  origin = factoryAPIURL("").replace(/\/$/, ""),
  readIdentity,
  list = listAgentMessages,
  follow,
}: MessageThreadOptions) {
  const scopeKey = JSON.stringify([origin, factorySessionID, workerSessionID]);
  const [selection, setSelection] = useState({
    scopeKey,
    member: 0,
    sent: false,
    threadId: null as string | null,
  });
  const current =
    selection.scopeKey === scopeKey
      ? selection
      : {
          scopeKey,
          member: 0,
          sent: false,
          threadId: null,
        };
  const active = enabled && workerSessionID !== null;
  const scope = useQuery({
    queryKey: [
      "agent-message-scope",
      origin,
      factorySessionID,
      workerSessionID,
    ],
    enabled: active,
    retry: false,
    refetchOnWindowFocus: false,
    queryFn: ({ signal }) =>
      readMessageScope(
        {
          origin,
          signal,
          workerSessionId: workerSessionID ?? "",
          factorySessionId: factorySessionID ?? undefined,
        },
        readIdentity,
      ),
  });
  const identity = scope.data?.[current.member];
  const page = useMessageFeed({
    origin,
    scopeKey,
    enabled: active && identity !== undefined,
    list,
    follow,
    filters: current.threadId
      ? { threadId: current.threadId }
      : current.sent
        ? { fromWorkerSessionId: identity?.workerSessionId }
        : {
            toWorkerSessionId: identity?.workerSessionId,
            factorySessionId: identity?.factorySessionId ?? undefined,
          },
  });
  const select = (patch: Partial<typeof selection>) =>
    setSelection({ ...current, ...patch });
  return {
    identities: scope.data ?? [],
    identity,
    messages: scopedMessagePage(page.messages, scope.data ?? []),
    threadId: current.threadId,
    sent: current.sent,
    busy: active && (scope.isFetching || page.busy),
    unavailable: !active,
    error: scope.error ?? page.error,
    previousAvailable: page.previousAvailable,
    nextAvailable: page.nextAvailable,
    live: page.live,
    newData: page.newData,
    setMember: (member: number) => select({ member, threadId: null }),
    setSent: (sent: boolean) => select({ sent, threadId: null }),
    setThread: (threadId: string | null) => select({ threadId }),
    previous: page.previous,
    next: page.next,
    refresh: () => {
      if (scope.isError) void scope.refetch();
      page.refresh();
    },
  };
}

export type MessageThreadState = ReturnType<typeof useMessageThread>;
export type NavigateMessageWorker = (identity: MessageWorkerIdentity) => void;
