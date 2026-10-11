import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { listAgentMessages } from "../../../api/agent-messages/api";
import {
  MessageReadError,
  type MessageWorkerIdentity,
} from "../../../api/agent-messages/types";
import { factoryAPIURL } from "../../../api/baseUrl";
import { scopedMessagePage } from "../lib/message-scope";
import {
  type MessageIdentityReader,
  readMessageScope,
} from "../lib/read-message-scope";

export interface MessageThreadOptions {
  factorySessionID: string | null;
  workerSessionID: string | null;
  enabled?: boolean;
  origin?: string;
  readIdentity?: MessageIdentityReader;
  list?: typeof listAgentMessages;
}

export function useMessageThread({
  factorySessionID,
  workerSessionID,
  enabled = true,
  origin = factoryAPIURL("").replace(/\/$/, ""),
  readIdentity,
  list = listAgentMessages,
}: MessageThreadOptions) {
  const scopeKey = JSON.stringify([origin, factorySessionID, workerSessionID]);
  const [selection, setSelection] = useState({
    scopeKey,
    member: 0,
    sent: false,
    threadId: null as string | null,
    cursors: [undefined] as (string | undefined)[],
  });
  const current =
    selection.scopeKey === scopeKey
      ? selection
      : {
          scopeKey,
          member: 0,
          sent: false,
          threadId: null,
          cursors: [undefined],
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
  const page = useQuery({
    queryKey: [
      "agent-message-thread",
      scopeKey,
      identity?.workerSessionId,
      identity?.factorySessionId,
      JSON.stringify(scope.data),
      current.member,
      current.sent,
      current.threadId,
      current.cursors.at(-1),
    ],
    enabled: active && identity !== undefined,
    retry: false,
    refetchOnWindowFocus: false,
    queryFn: async ({ signal }) => {
      if (!identity) throw new MessageReadError("unavailable");
      const result = await list({
        origin,
        signal,
        nextToken: current.cursors.at(-1),
        ...(current.threadId
          ? { threadId: current.threadId }
          : current.sent
            ? { fromWorkerSessionId: identity.workerSessionId }
            : {
                toWorkerSessionId: identity.workerSessionId,
                factorySessionId: identity.factorySessionId,
              }),
      });
      return {
        ...result,
        messages: scopedMessagePage(result.messages, scope.data ?? []),
      };
    },
  });
  const select = (patch: Partial<typeof selection>) =>
    setSelection({ ...current, ...patch, cursors: [undefined] });
  return {
    identities: scope.data ?? [],
    identity,
    messages: page.data?.messages ?? [],
    threadId: current.threadId,
    sent: current.sent,
    busy: active && (scope.isFetching || page.isFetching),
    unavailable: !active,
    error: scope.error ?? page.error,
    previousAvailable: current.cursors.length > 1,
    nextAvailable: Boolean(page.data?.nextToken),
    setMember: (member: number) => select({ member, threadId: null }),
    setSent: (sent: boolean) => select({ sent, threadId: null }),
    setThread: (threadId: string | null) => select({ threadId }),
    previous: () =>
      setSelection({ ...current, cursors: current.cursors.slice(0, -1) }),
    next: () => {
      if (page.data?.nextToken)
        setSelection({
          ...current,
          cursors: [...current.cursors, page.data.nextToken],
        });
    },
    refresh: () => {
      select({});
      if (scope.isError) void scope.refetch();
      else if (current.cursors.length === 1) void page.refetch();
    },
  };
}

export type MessageThreadState = ReturnType<typeof useMessageThread>;
export type NavigateMessageWorker = (identity: MessageWorkerIdentity) => void;
