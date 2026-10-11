import { expect, it } from "bun:test";
import type { AgentMessageObservation } from "../../../api/agent-messages/types";
import { reconcileMessagePage, retainMessageObservation } from "./message-page";
import { messageFixture } from "./testing/message-fixture";

it("keeps server send order and immutable associations while suppressing stale per-message transitions", () => {
  const first = messageFixture({ messageId: "z", inReplyTo: "outside-page" });
  const second = messageFixture({ messageId: "a" });
  const updates = new Map<string, AgentMessageObservation>();
  const observation = (
    message: typeof first,
    sequence: number,
  ): AgentMessageObservation => ({
    recordId: `${message.messageId}-${sequence}`,
    sequence,
    kind: "READ",
    message,
  });
  expect(
    retainMessageObservation(
      updates,
      observation(
        {
          ...first,
          status: "READ",
          sentAt: "2026-10-12T00:00:00Z",
          threadId: "wrong",
        },
        9,
      ),
    ),
  ).toBe(true);
  expect(
    retainMessageObservation(
      updates,
      observation({ ...second, status: "REPLIED" }, 9),
    ),
  ).toBe(true);
  expect(retainMessageObservation(updates, observation(first, 8))).toBe(false);
  expect(retainMessageObservation(updates, observation(first, 9))).toBe(false);
  const rows = reconcileMessagePage([first, second, first], updates, {});
  expect(rows.map((row) => [row.messageId, row.status])).toEqual([
    ["z", "READ"],
    ["a", "REPLIED"],
  ]);
  expect(rows[0].sentAt).toBe(first.sentAt);
  expect(rows[0].threadId).toBe(first.threadId);
  expect(rows[0].inReplyTo).toBe("outside-page");
  expect(
    reconcileMessagePage([first, second], updates, { status: ["QUEUED"] }),
  ).toEqual([]);
});
