import { describe, expect, it } from "bun:test";
import {
  MessageReadError,
  type MessageWorkerIdentity,
} from "../../../api/agent-messages/types";
import {
  getAgentMessageMessages,
  messageRelativeTime,
} from "../messages/agent-messages";
import {
  confirmedMessageSuccessor,
  messageTouchesScope,
  scopedMessagePage,
} from "./message-scope";
import {
  readConfirmedMessageSuccessor,
  readMessageScope,
} from "./read-message-scope";
import { messageFixture } from "./testing/message-fixture";

describe("exact message scope projection", () => {
  it("preserves server send order, reply parent and equal timestamps, deduping record identity", () => {
    const first = messageFixture({ messageId: "msg-z" });
    const second = messageFixture({
      messageId: "msg-a",
      inReplyTo: "msg-outside-page",
    });
    const scope = [
      { workerSessionId: "worker", factorySessionId: "factory-a" },
    ];
    const rows = scopedMessagePage([first, second, first], scope);
    expect(rows.map((row) => row.messageId)).toEqual(["msg-z", "msg-a"]);
    expect(rows[1].inReplyTo).toBe("msg-outside-page");
  });

  it("ignores labels and correlation when endpoint Factory Session differs", () => {
    const row = messageFixture({
      correlation: { factorySessionId: "factory-x" },
    });
    expect(
      messageTouchesScope(row, [
        { workerSessionId: "worker", factorySessionId: "factory-x" },
      ]),
    ).toBe(false);
    expect(messageTouchesScope(row, [{ workerSessionId: "worker" }])).toBe(
      false,
    );
    expect(
      messageTouchesScope(row, [
        { workerSessionId: "worker", factorySessionId: "factory-a" },
      ]),
    ).toBe(true);
  });

  it("reads a continuation's own scope and rejects missing or unverified linkage", async () => {
    const calls: { workerSessionId: string; factorySessionId?: string }[] = [];
    const reader = async (options: {
      workerSessionId: string;
      factorySessionId?: string;
    }): Promise<MessageWorkerIdentity> => {
      calls.push(options);
      return options.workerSessionId === "worker"
        ? {
            workerSessionId: "worker",
            factorySessionId: "factory-a",
            successorWorkerSessionId: "next",
          }
        : { workerSessionId: "next", predecessorWorkerSessionId: "worker" };
    };
    const scope = await readMessageScope(
      {
        origin: "host",
        workerSessionId: "worker",
        factorySessionId: "factory-a",
      },
      reader,
    );
    expect(scope[1].factorySessionId).toBeUndefined();
    expect(calls[1].factorySessionId).toBeUndefined();
    await expect(
      readMessageScope(
        {
          origin: "host",
          workerSessionId: "worker",
          factorySessionId: "other",
        },
        reader,
      ),
    ).rejects.toEqual(new MessageReadError("unavailable"));
    await expect(
      readMessageScope(
        {
          origin: "host",
          workerSessionId: "worker",
          factorySessionId: "factory-a",
        },
        async (options) => ({
          workerSessionId: options.workerSessionId,
          factorySessionId: "factory-a",
          successorWorkerSessionId: "unrelated",
        }),
      ),
    ).rejects.toEqual(new MessageReadError("unavailable"));
  });

  it("enables only confirmed matching delivery fields, and validates successor navigation against the recipient", async () => {
    const row = messageFixture({
      status: "DELIVERED_REVIVE",
      revivedWorkerSessionId: "revived",
      to: { ...messageFixture().to, deliveredWorkerSessionId: "revived" },
    });
    expect(
      confirmedMessageSuccessor({
        ...row,
        status: "QUEUED",
        reason: "REVIVE_PENDING",
      }),
    ).toBeNull();
    expect(
      confirmedMessageSuccessor({ ...row, revivedWorkerSessionId: "wrong" }),
    ).toBeNull();
    const identity = await readConfirmedMessageSuccessor(
      row,
      { origin: "host" },
      async (options) =>
        options.workerSessionId === "lead"
          ? {
              workerSessionId: "lead",
              factorySessionId: "factory-b",
              successorWorkerSessionId: "revived",
            }
          : { workerSessionId: "revived", predecessorWorkerSessionId: "lead" },
    );
    expect(identity).toEqual({
      workerSessionId: "revived",
      predecessorWorkerSessionId: "lead",
    });
  });

  it("formats age and expiry in English/Spanish/Chinese without changing status", () => {
    const row = messageFixture();
    const now = Date.parse("2026-10-11T12:01:00Z");
    expect(messageRelativeTime(row.sentAt, now, "en")).toBe("1 minute ago");
    expect(messageRelativeTime(row.sentAt, now, "es")).toBe("hace 1 minuto");
    expect(messageRelativeTime(row.sentAt, now, "zh-CN")).toBe("1分钟前");
    expect(getAgentMessageMessages("es").status.QUEUED).toBe("En cola");
    expect(row.status).toBe("QUEUED");
  });
});
