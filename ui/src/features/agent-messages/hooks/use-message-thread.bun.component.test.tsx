import { describe, expect, it } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import type {
  MessageListOptions,
  MessageWorkerIdentity,
} from "../../../api/agent-messages/types";
import { MessageReadError } from "../../../api/agent-messages/types";
import { messageFixture as baseMessageFixture } from "../lib/testing/message-fixture";
import { useMessageThread } from "./use-message-thread";

function messageFixture(
  overrides: Parameters<typeof baseMessageFixture>[0] = {},
) {
  return baseMessageFixture({
    to: {
      kind: "WORKER_SESSION",
      workerSessionId: "worker",
      factorySessionId: "factory-a",
    },
    ...overrides,
  });
}

function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

const readIdentity = async (options: {
  workerSessionId: string;
  factorySessionId?: string;
}): Promise<MessageWorkerIdentity> => ({
  workerSessionId: options.workerSessionId,
  factorySessionId: options.factorySessionId,
});

describe("message thread query ownership", () => {
  it("keeps one cursor page, navigates both directions, resets the walk for thread and send filters", async () => {
    const requests: MessageListOptions[] = [];
    const list = async (options: MessageListOptions) => {
      requests.push(options);
      return options.nextToken
        ? {
            messages: [
              messageFixture({
                messageId: "msg-next",
                inReplyTo: "msg-outside",
              }),
            ],
          }
        : { messages: [messageFixture()], nextToken: "page-two" };
    };
    const { result } = renderHook(
      () =>
        useMessageThread({
          origin: "host-a",
          factorySessionID: "factory-a",
          workerSessionID: "worker",
          readIdentity,
          follow: async () => {},
          list,
        }),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.messages).toHaveLength(1));
    expect(requests[0].toWorkerSessionId).toBe("worker");
    act(() => result.current.next());
    await waitFor(() =>
      expect(result.current.messages[0]?.messageId).toBe("msg-next"),
    );
    expect(result.current.messages[0].inReplyTo).toBe("msg-outside");
    expect(result.current.previousAvailable).toBe(true);
    expect(requests.at(-1)?.nextToken).toBe("page-two");
    act(() => result.current.previous());
    await waitFor(() =>
      expect(result.current.messages[0]?.messageId).toBe("msg-question"),
    );
    act(() => result.current.setThread("msg-question"));
    await waitFor(() => expect(requests.at(-1)?.threadId).toBe("msg-question"));
    expect(requests.at(-1)?.toWorkerSessionId).toBeUndefined();
    expect(result.current.previousAvailable).toBe(false);
    act(() => result.current.setSent(true));
    await waitFor(() =>
      expect(requests.at(-1)?.fromWorkerSessionId).toBe("worker"),
    );
    expect(requests.at(-1)?.threadId).toBeUndefined();
  });
});

describe("message thread cancellation and recovery", () => {
  it("aborts the old host generation and cannot publish its late response into a new tuple", async () => {
    let oldRequest: MessageListOptions | undefined;
    let finishOld:
      | ((page: { messages: ReturnType<typeof messageFixture>[] }) => void)
      | undefined;
    const list = (options: MessageListOptions) => {
      if (options.origin === "host-a") {
        oldRequest = options;
        return new Promise<{ messages: ReturnType<typeof messageFixture>[] }>(
          (resolve) => {
            finishOld = resolve;
          },
        );
      }
      return Promise.resolve({
        messages: [
          messageFixture({
            messageId: "new-host",
            to: {
              kind: "WORKER_SESSION",
              workerSessionId: "worker",
              factorySessionId: "factory-b",
            },
          }),
        ],
      });
    };
    const { result, rerender, unmount } = renderHook(
      ({ origin, factorySessionID }) =>
        useMessageThread({
          origin,
          factorySessionID,
          workerSessionID: "worker",
          readIdentity,
          follow: async () => {},
          list,
        }),
      {
        initialProps: { origin: "host-a", factorySessionID: "factory-a" },
        wrapper: wrapper(),
      },
    );
    await waitFor(() => expect(oldRequest).toBeDefined());
    rerender({ origin: "host-b", factorySessionID: "factory-b" });
    await waitFor(() =>
      expect(result.current.messages[0]?.messageId).toBe("new-host"),
    );
    expect(oldRequest?.signal?.aborted).toBe(true);
    await act(async () => {
      finishOld?.({ messages: [messageFixture()] });
    });
    expect(result.current.messages.map((row) => row.messageId)).toEqual([
      "new-host",
    ]);
    unmount();
  });

  it("restarts an expired cursor at the first page and keeps errors typed", async () => {
    const requests: MessageListOptions[] = [];
    const list = async (options: MessageListOptions) => {
      requests.push(options);
      if (options.nextToken) throw new MessageReadError("expired-page");
      return { messages: [messageFixture()], nextToken: "expired" };
    };
    const { result } = renderHook(
      () =>
        useMessageThread({
          origin: "host",
          factorySessionID: "factory-a",
          workerSessionID: "worker",
          readIdentity,
          follow: async () => {},
          list,
        }),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.nextAvailable).toBe(true));
    act(() => result.current.next());
    await waitFor(() =>
      expect(result.current.error).toEqual(
        new MessageReadError("expired-page"),
      ),
    );
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.messages).toHaveLength(1));
    expect(result.current.previousAvailable).toBe(false);
    expect(requests.at(-1)?.nextToken).toBeUndefined();
  });

  it("starts no reads for an absent selection or disabled surface", async () => {
    let calls = 0;
    const list = async () => {
      calls++;
      return { messages: [] };
    };
    const { result } = renderHook(
      () =>
        useMessageThread({
          origin: "host",
          factorySessionID: null,
          workerSessionID: "worker",
          enabled: false,
          readIdentity,
          follow: async () => {},
          list,
        }),
      { wrapper: wrapper() },
    );
    expect(result.current.unavailable).toBe(true);
    expect(calls).toBe(0);
  });
});
