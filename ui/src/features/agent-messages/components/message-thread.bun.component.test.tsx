import { describe, expect, it, mock } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type {
  AgentMessage,
  MessageReadFailure,
} from "../../../api/agent-messages/types";
import { MessageReadError } from "../../../api/agent-messages/types";
import type { MessageThreadState } from "../hooks/use-message-thread";
import { messageFixture } from "../lib/testing/message-fixture";
import { getAgentMessageMessages } from "../messages/agent-messages";
import { MessageThread, MessageThreadContent } from "./message-thread";

function state(
  overrides: Partial<MessageThreadState> = {},
): MessageThreadState {
  return {
    identities: [],
    identity: undefined,
    messages: [],
    threadId: null,
    sent: false,
    busy: false,
    unavailable: false,
    error: null,
    previousAvailable: false,
    nextAvailable: false,
    setMember: () => {},
    setSent: () => {},
    setThread: () => {},
    previous: () => {},
    next: () => {},
    refresh: () => {},
    ...overrides,
  };
}

describe("read-only message thread presentation", () => {
  it("renders normalized plain text, reply associations, distinct status/reasons and confirmed successor actions", () => {
    const onThread = mock(() => {});
    const onSuccessor = mock(() => {});
    const rows: AgentMessage[] = [
      messageFixture({
        body: "<script>alert('unsafe')</script> [redacted]",
        reason: "REVIVE_PENDING",
      }),
      messageFixture({
        messageId: "msg-reply",
        inReplyTo: "msg-question",
        status: "REPLIED",
        body: "Hold reply",
      }),
      messageFixture({
        messageId: "msg-delivered",
        status: "DELIVERED_REVIVE",
        revivedWorkerSessionId: "revived",
        to: { ...messageFixture().to, deliveredWorkerSessionId: "revived" },
      }),
      messageFixture({ messageId: "msg-read", status: "READ" }),
      messageFixture({ messageId: "msg-expired", status: "EXPIRED" }),
    ];
    const { container } = render(
      <MessageThreadContent
        state={state({ messages: rows, setThread: onThread })}
        onSuccessor={onSuccessor}
      />,
    );
    expect(screen.getByText(rows[0].body)).toBeTruthy();
    expect(container.querySelector("script")).toBeNull();
    for (const status of [
      "Queued",
      "Replied",
      "Delivered by revival",
      "Read",
      "Expired",
    ])
      expect(screen.getByText(status)).toBeTruthy();
    expect(screen.getByText("Revival pending")).toBeTruthy();
    expect(screen.getByText("msg-question")).toBeTruthy();
    expect(container.querySelectorAll("time")).toHaveLength(rows.length * 2);
    fireEvent.click(
      screen.getAllByRole("button", { name: "Open thread msg-question" })[0],
    );
    expect(onThread).toHaveBeenCalledWith("msg-question");
    const links = screen.getAllByRole("button", {
      name: "Open revived Worker Session revived",
    });
    expect(links).toHaveLength(1);
    fireEvent.click(links[0]);
    expect(onSuccessor).toHaveBeenCalledWith(rows[2]);
  });
});

describe("message thread delivery reasons", () => {
  it.each([
    "REVIVE_PENDING",
    "REVIVE_UNSUPPORTED",
    "REVIVE_FAILED",
    "REVIVE_LIMIT",
  ])("renders queued %s without a successor action", (reason) => {
    render(
      <MessageThreadContent
        state={state({ messages: [messageFixture({ reason })] })}
        onSuccessor={() => {}}
      />,
    );
    expect(
      screen.getByText(getAgentMessageMessages().reasons[reason]),
    ).toBeTruthy();
    expect(screen.getByText("Queued")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /Open revived Worker Session/ }),
    ).toBeNull();
  });
});

describe("message thread recovery and localization", () => {
  it.each([
    "disabled",
    "denied",
    "unavailable",
    "invalid",
    "expired-page",
    "error",
  ] as MessageReadFailure[])("shows safe actionable %s failures", (code) => {
    const retry = mock(() => {});
    render(
      <MessageThreadContent
        state={state({ error: new MessageReadError(code), refresh: retry })}
      />,
    );
    expect(screen.getByRole("status").textContent).toBe(
      getAgentMessageMessages().failures[code],
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("distinguishes pending, empty and no-selection states and keeps paging keyboard accessible", () => {
    const { rerender } = render(
      <MessageThreadContent state={state({ busy: true })} />,
    );
    expect(screen.getByRole("status").textContent).toBe("Loading messages…");
    rerender(<MessageThreadContent state={state()} />);
    expect(screen.getByRole("status").textContent).toBe(
      "No messages in this page.",
    );
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
    rerender(<MessageThreadContent state={state({ unavailable: true })} />);
    expect(screen.queryByRole("button", { name: "Next" })).toBeNull();
    expect(screen.getByRole("status").textContent).toContain(
      "Select an available Worker Session",
    );
  });

  it("keeps the source recoverable when confirmed successor inspection fails", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
    const navigate = mock(() => {});
    const delivered = messageFixture({
      status: "DELIVERED_REVIVE",
      revivedWorkerSessionId: "revived",
      to: { ...messageFixture().to, deliveredWorkerSessionId: "revived" },
    });
    render(
      <QueryClientProvider client={client}>
        <MessageThread
          origin="host"
          factorySessionID="factory-a"
          workerSessionID="worker"
          list={async () => ({ messages: [delivered] })}
          readIdentity={async (options) => {
            if (options.workerSessionId !== "worker")
              throw new Error("private provider path and credential");
            return { workerSessionId: "worker", factorySessionId: "factory-a" };
          }}
          onNavigate={navigate}
        />
      </QueryClientProvider>,
    );
    fireEvent.click(
      await screen.findByRole("button", {
        name: "Open revived Worker Session revived",
      }),
    );
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toBe(
        getAgentMessageMessages().successorError,
      ),
    );
    expect(navigate).not.toHaveBeenCalled();
    expect(
      within(screen.getByRole("region", { name: "Messages" })).getByText(
        delivered.body,
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/private provider path/)).toBeNull();
  });

  it("exposes Spanish labels through the feature catalog", () => {
    render(
      <MessageThreadContent
        locale="es"
        state={state({
          messages: [messageFixture({ reason: "REVIVE_UNSUPPORTED" })],
        })}
      />,
    );
    expect(screen.getByRole("region", { name: "Mensajes" })).toBeTruthy();
    expect(screen.getByText("Reactivación no compatible")).toBeTruthy();
  });
});
