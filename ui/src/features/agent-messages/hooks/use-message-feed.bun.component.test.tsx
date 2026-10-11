import { expect, it } from "bun:test";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { followAgentMessages } from "../../../api/agent-messages/follow";
import {
  type AgentMessageObservation,
  type MessageListOptions,
  MessageReadError,
} from "../../../api/agent-messages/types";
import { messageFixture } from "../lib/testing/message-fixture";
import { useMessageFeed } from "./use-message-feed";

function subscription() {
  let receive: (value: AgentMessageObservation) => void = () => {};
  let fail: (error: unknown) => void = () => {};
  const requests: Parameters<typeof followAgentMessages>[0][] = [];
  const follow: typeof followAgentMessages = (options, onObservation) => {
    requests.push(options);
    receive = onObservation;
    return new Promise<void>((resolve, reject) => {
      fail = reject;
      options.signal?.addEventListener("abort", () => resolve(), {
        once: true,
      });
    });
  };
  return {
    follow,
    requests,
    emit: (message: ReturnType<typeof messageFixture>, sequence: number) =>
      receive({
        recordId: `${message.messageId}-${sequence}`,
        sequence,
        kind: "DELIVERY_UPDATED",
        message,
      }),
    fail: (error: unknown) => fail(error),
  };
}

it("starts retained reads before idle stream headers, preserves newer concurrent observations and send order", async () => {
  const stream = subscription();
  let finish:
    | ((value: { messages: ReturnType<typeof messageFixture>[] }) => void)
    | undefined;
  let reads = 0;
  const rows = [
    messageFixture({ messageId: "z" }),
    messageFixture({ messageId: "a" }),
  ];
  const list = async () => {
    reads++;
    if (reads === 1)
      return new Promise<{ messages: typeof rows }>((resolve) => {
        finish = resolve;
      });
    return { messages: rows };
  };
  const { result, unmount } = renderHook(() =>
    useMessageFeed({ origin: "host", list, follow: stream.follow }),
  );
  expect(reads).toBe(1);
  act(() => {
    stream.emit({ ...rows[0], status: "READ" }, 12);
    stream.emit({ ...rows[1], status: "REPLIED" }, 12);
    stream.emit(rows[0], 11);
  });
  await act(async () => {
    finish?.({ messages: rows });
  });
  await waitFor(() => expect(result.current.busy).toBe(false));
  expect(reads).toBe(2);
  expect(
    result.current.messages.map((row) => [row.messageId, row.status]),
  ).toEqual([
    ["z", "READ"],
    ["a", "REPLIED"],
  ]);
  act(() => stream.emit(messageFixture({ messageId: "new-send" }), 50));
  expect(result.current.messages).toHaveLength(2);
  expect(result.current.newData).toBe(true);
  expect(result.current.error).toBeNull(); // Filtered sequence gaps are not retention gaps.
  unmount();
  expect(stream.requests[0].signal?.aborted).toBe(true);
});

it("observes transitions out of status filters, bounds pages, reloads after a gap and aborts old hosts", async () => {
  const stream = subscription();
  const requests: MessageListOptions[] = [];
  const list = async (options: MessageListOptions) => {
    requests.push(options);
    return {
      messages: [
        messageFixture({ messageId: options.nextToken ? "page-two" : "first" }),
      ],
      nextToken: options.nextToken ? undefined : "second",
    };
  };
  const { result, rerender, unmount } = renderHook(
    ({ origin }) =>
      useMessageFeed({
        origin,
        filters: { status: ["QUEUED"] },
        list,
        follow: stream.follow,
      }),
    { initialProps: { origin: "host-a" } },
  );
  await waitFor(() => expect(result.current.messages).toHaveLength(1));
  expect(stream.requests[0].status).toBeUndefined();
  act(() =>
    stream.emit(messageFixture({ messageId: "first", status: "READ" }), 2),
  );
  await waitFor(() => expect(result.current.messages).toHaveLength(0));
  act(() => result.current.next());
  await waitFor(() =>
    expect(result.current.messages[0]?.messageId).toBe("page-two"),
  );
  expect(requests.at(-1)?.nextToken).toBe("second");
  act(() => result.current.previous());
  await waitFor(() =>
    expect(result.current.messages[0]?.messageId).toBe("first"),
  );
  act(() => stream.fail(new MessageReadError("gap")));
  await waitFor(() => expect(result.current.error?.code).toBe("gap"));
  expect(stream.requests.at(-1)?.signal?.aborted).toBe(true);
  act(() => result.current.refresh());
  await waitFor(() => expect(result.current.error).toBeNull());
  expect(requests.at(-1)?.nextToken).toBeUndefined();
  const old = stream.requests.at(-1);
  rerender({ origin: "host-b" });
  await waitFor(() => expect(requests.at(-1)?.origin).toBe("host-b"));
  expect(old?.signal?.aborted).toBe(true);
  unmount();
});

it("closes subscriptions when a retained read is denied and makes no disabled reads", async () => {
  const stream = subscription();
  let calls = 0;
  const list = async () => {
    calls++;
    throw new MessageReadError("denied");
  };
  const { result, rerender } = renderHook(
    ({ enabled }) =>
      useMessageFeed({ origin: "host", enabled, list, follow: stream.follow }),
    { initialProps: { enabled: false } },
  );
  expect(calls).toBe(0);
  expect(stream.requests).toHaveLength(0);
  rerender({ enabled: true });
  await waitFor(() => expect(result.current.error?.code).toBe("denied"));
  expect(stream.requests[0].signal?.aborted).toBe(true);
});

it("keeps stale rows during recovery, replaces subscriptions on online transition and fences late old-host responses", async () => {
  const stream = subscription();
  let reads = 0;
  let finish:
    | ((page: { messages: ReturnType<typeof messageFixture>[] }) => void)
    | undefined;
  const list = async (options: MessageListOptions) => {
    reads++;
    if (options.origin === "host-b")
      return { messages: [messageFixture({ messageId: "host-b" })] };
    if (reads > 1)
      return new Promise<{ messages: ReturnType<typeof messageFixture>[] }>(
        (resolve) => {
          finish = resolve;
        },
      );
    return { messages: [messageFixture({ messageId: "stale" })] };
  };
  const { result, rerender, unmount } = renderHook(
    ({ origin }) => useMessageFeed({ origin, list, follow: stream.follow }),
    { initialProps: { origin: "host-a" } },
  );
  await waitFor(() =>
    expect(result.current.messages[0]?.messageId).toBe("stale"),
  );
  const oldStream = stream.requests[0];
  act(() => window.dispatchEvent(new Event("online")));
  await waitFor(() => expect(reads).toBe(2));
  expect(oldStream.signal?.aborted).toBe(true);
  expect(result.current.busy).toBe(true);
  expect(result.current.messages[0].messageId).toBe("stale");
  rerender({ origin: "host-b" });
  await waitFor(() =>
    expect(result.current.messages[0]?.messageId).toBe("host-b"),
  );
  await act(async () =>
    finish?.({ messages: [messageFixture({ messageId: "late-old-host" })] }),
  );
  expect(result.current.messages[0].messageId).toBe("host-b");
  unmount();
});
