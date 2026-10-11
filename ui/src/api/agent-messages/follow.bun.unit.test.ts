import { expect, it } from "bun:test";
import { messageFixture } from "../../features/agent-messages/lib/testing/message-fixture";
import { followAgentMessages } from "./follow";
import type { AgentMessageObservation } from "./types";
import { MessageReadError } from "./types";

it("reads split named SSE frames without credentials, status filtering, cursors or mutation; abort closes its body", async () => {
  const controller = new AbortController();
  const observations: AgentMessageObservation[] = [];
  let cancelled = false;
  const frame = `event: message\r\ndata: ${JSON.stringify({ recordId: "txn", sequence: 6, kind: "READ", message: messageFixture({ status: "READ" }) })}\r\n\r\n`;
  const fetcher = (async (url, init) => {
    const query = new URL(String(url)).searchParams;
    expect(Object.fromEntries(query)).toEqual({
      maxResults: "100",
      toWorkerSessionId: "lead",
      follow: "true",
    });
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("omit");
    expect(init?.signal).toBe(controller.signal);
    return new Response(
      new ReadableStream({
        start(stream) {
          for (let i = 0; i < frame.length; i += 7)
            stream.enqueue(new TextEncoder().encode(frame.slice(i, i + 7)));
        },
        cancel() {
          cancelled = true;
        },
      }),
      { headers: { "Content-Type": "text/event-stream" } },
    );
  }) as typeof fetch;
  await followAgentMessages(
    {
      origin: "http://selected",
      toWorkerSessionId: "lead",
      status: ["QUEUED"],
      signal: controller.signal,
      fetch: fetcher,
    },
    (observation) => {
      observations.push(observation);
      controller.abort();
    },
  );
  expect(observations.map((row) => row.message.status)).toEqual(["READ"]);
  expect(cancelled).toBe(true);
});

it.each([
  ["event: message\ndata: {invalid}\n\n", "invalid"],
  [
    'event: error\ndata: {"code":"MESSAGE_STREAM_GAP","message":"secret"}\n\n',
    "gap",
  ],
] as const)(
  "closes failed streams and returns safe %s errors",
  async (frame, code) => {
    let cancelled = false;
    await expect(
      followAgentMessages(
        {
          origin: "host",
          fetch: (async () =>
            new Response(
              new ReadableStream({
                start(stream) {
                  stream.enqueue(new TextEncoder().encode(frame));
                },
                cancel() {
                  cancelled = true;
                },
              }),
              { headers: { "Content-Type": "text/event-stream" } },
            )) as typeof fetch,
        },
        () => {},
      ),
    ).rejects.toEqual(new MessageReadError(code));
    expect(cancelled).toBe(true);
  },
);
