import type { AgentMessage } from "../../../../api/agent-messages/types";

export function messageFixture(
  overrides: Partial<AgentMessage> = {},
): AgentMessage {
  return {
    messageId: "msg-question",
    threadId: "msg-question",
    from: {
      principal: "WORKER",
      workerSessionId: "worker",
      factorySessionId: "factory-a",
    },
    to: {
      kind: "WORKER_SESSION",
      workerSessionId: "lead",
      factorySessionId: "factory-b",
    },
    body: "[redacted] Can I continue?",
    bodySha256: "normalized-hash",
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
