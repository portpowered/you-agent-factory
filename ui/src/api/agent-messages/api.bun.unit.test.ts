import { describe, expect, it } from "bun:test";
import { listAgentMessages, readMessageWorkerIdentity } from "./api";
import { parseAgentMessagePage } from "./parsing";
import { type AgentMessage, MessageReadError } from "./types";

function message(overrides: Partial<AgentMessage> = {}): AgentMessage {
  return {
    messageId: "msg-first",
    threadId: "msg-first",
    from: {
      principal: "WORKER",
      workerSessionId: "sender",
      factorySessionId: "factory-a",
    },
    to: {
      kind: "WORKER_SESSION",
      workerSessionId: "recipient",
      factorySessionId: "factory-b",
    },
    body: "[redacted] <script>private text</script>",
    bodySha256: "redacted-hash",
    bodyRedactionCount: 1,
    delivery: "QUEUE",
    ifEnded: "HOLD",
    replyIfEnded: "HOLD",
    status: "QUEUED",
    hop: 0,
    sentAt: "2026-10-11T12:00:00Z",
    expiresAt: "2026-10-12T12:00:00Z",
    ...overrides,
  };
}

describe("operator message reader", () => {
  it("uses a captured host, repeated AND filters and a bounded cursor GET without credentials or mutations", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    const fetcher = (async (url, init) => {
      calls.push({ url: String(url), init });
      return Response.json({ messages: [message()], nextToken: "next/opaque" });
    }) as typeof fetch;
    const controller = new AbortController();
    const page = await listAgentMessages({
      origin: "https://selected.example",
      factorySessionId: "factory-b",
      toWorkerSessionId: "recipient",
      fromWorkerSessionId: "sender",
      threadId: "msg-first",
      status: ["QUEUED", "READ"],
      nextToken: "cursor/+",
      fetch: fetcher,
      signal: controller.signal,
    });
    const url = new URL(calls[0].url);
    expect(url.origin).toBe("https://selected.example");
    expect(url.pathname).toBe("/messages");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      maxResults: "100",
      factorySessionId: "factory-b",
      toWorkerSessionId: "recipient",
      fromWorkerSessionId: "sender",
      threadId: "msg-first",
      nextToken: "cursor/+",
      status: "READ",
    });
    expect(url.searchParams.getAll("status")).toEqual(["QUEUED", "READ"]);
    expect(calls[0].init).toEqual({
      method: "GET",
      credentials: "omit",
      signal: controller.signal,
    });
    expect(page.messages[0].body).toBe(message().body);
    expect(page.nextToken).toBe("next/opaque");
  });

  it("reads explicit Worker Session identity separately from informational correlation", async () => {
    const fetcher = (async (url) => {
      expect(String(url)).toBe(
        "http://host/worker-sessions/worker%2Fa?factorySessionId=factory-a",
      );
      return Response.json({
        workerSessionId: "worker/a",
        factorySessionId: "factory-a",
        correlation: { factorySessionId: "wrong-factory" },
      });
    }) as typeof fetch;
    expect(
      (
        await readMessageWorkerIdentity({
          origin: "http://host",
          workerSessionId: "worker/a",
          factorySessionId: "factory-a",
          fetch: fetcher,
        })
      ).factorySessionId,
    ).toBe("factory-a");
  });
});

describe("operator message reader recovery", () => {
  it("propagates cancellation to its owned read without returning private network errors", async () => {
    const controller = new AbortController();
    const fetcher = ((_url, init) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener(
          "abort",
          () => reject(new Error("token=private")),
          { once: true },
        );
      })) as typeof fetch;
    const pending = listAgentMessages({
      origin: "http://host",
      fetch: fetcher,
      signal: controller.signal,
    });
    controller.abort();
    await expect(pending).rejects.toEqual(new MessageReadError("unavailable"));
  });

  it("keeps an ambiguous Worker Session unavailable instead of treating it as a message cursor", async () => {
    const fetcher = (async () =>
      Response.json(
        { code: "WORKER_SESSION_AMBIGUOUS", message: "private diagnostics" },
        { status: 409 },
      )) as typeof fetch;
    await expect(
      readMessageWorkerIdentity({
        origin: "host",
        workerSessionId: "duplicate",
        fetch: fetcher,
      }),
    ).rejects.toEqual(new MessageReadError("unavailable"));
  });

  it.each([
    [403, "denied"],
    [409, "expired-page"],
    [503, "unavailable"],
    [500, "error"],
  ] as const)(
    "normalizes HTTP %s without retaining diagnostics",
    async (status, code) => {
      const fetcher = (async () =>
        Response.json(
          { message: "Authorization: secret", path: "private/session" },
          { status },
        )) as typeof fetch;
      await expect(
        listAgentMessages({ origin: "http://host", fetch: fetcher }),
      ).rejects.toEqual(new MessageReadError(code));
    },
  );

  it("distinguishes disabled messaging and rejects malformed public responses", async () => {
    await expect(
      listAgentMessages({
        origin: "",
        fetch: (async () =>
          Response.json(
            { code: "MESSAGING_DISABLED", message: "secret" },
            { status: 503 },
          )) as typeof fetch,
      }),
    ).rejects.toEqual(new MessageReadError("disabled"));
    for (const invalid of [
      { messages: {} },
      { messages: [message({ status: "MADE_UP" as AgentMessage["status"] })] },
      { messages: [message({ expiresAt: "secret" })] },
      {
        messages: [
          message({ to: { kind: "WORKER_SESSION", workerSessionId: "" } }),
        ],
      },
    ]) {
      expect(() => parseAgentMessagePage(invalid)).toThrow(
        new MessageReadError("invalid"),
      );
    }
  });
});
