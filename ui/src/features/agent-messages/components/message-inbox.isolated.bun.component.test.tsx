import { expect, it, mock } from "bun:test";
import { fireEvent, render, screen } from "@testing-library/react";
import { MessageReadError } from "../../../api/agent-messages/types";
import type {
  MessageFeedOptions,
  MessageFeedState,
} from "../hooks/use-message-feed";
import { messageFixture } from "../lib/testing/message-fixture";

let options: MessageFeedOptions;
const refresh = mock(() => {});
const next = mock(() => {});
let feed: MessageFeedState = {
  messages: [messageFixture()],
  busy: false,
  error: null,
  live: true,
  newData: false,
  unavailable: false,
  nextAvailable: true,
  previousAvailable: false,
  refresh,
  next,
  previous: () => {},
};
mock.module("../hooks/use-message-feed", () => ({
  useMessageFeed: (value: MessageFeedOptions) => {
    options = value;
    return feed;
  },
}));
mock.module("../hooks/use-message-successor", () => ({
  useMessageSuccessor: () => ({ error: false, open: undefined }),
}));
mock.module("../../../components/ui/enum-select", () => ({
  OptionalEnumSelect: ({
    id,
    value,
    options,
    emptyOptionLabel,
    onValueChange,
  }: {
    id: string;
    value: string | null;
    options: { value: string; label: string }[];
    emptyOptionLabel: string;
    onValueChange: (value: string | null) => void;
  }) => (
    <select
      id={id}
      value={value ?? ""}
      onChange={(event) => onValueChange(event.target.value || null)}
    >
      <option value="">{emptyOptionLabel}</option>
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
}));
const { MessageInbox } = await import("./message-inbox");

it("labels read-only filters, applies bounded paging, clears status for thread inspection and announces safe recovery", () => {
  const { rerender } = render(<MessageInbox origin="http://selected" />);
  fireEvent.change(screen.getByLabelText("Recipient Worker Session"), {
    target: { value: "lead" },
  });
  fireEvent.change(screen.getByLabelText("Sender Worker Session"), {
    target: { value: "worker" },
  });
  fireEvent.change(screen.getByLabelText("Status"), {
    target: { value: "QUEUED" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));
  expect(options.filters).toEqual({
    toWorkerSessionId: "lead",
    fromWorkerSessionId: "worker",
    threadId: undefined,
    status: ["QUEUED"],
  });
  fireEvent.click(screen.getByRole("button", { name: "Next", exact: true }));
  expect(next).toHaveBeenCalledTimes(1);
  fireEvent.click(
    screen.getByRole("button", { name: "Open thread msg-question" }),
  );
  expect(options.filters).toEqual({ threadId: "msg-question" });
  fireEvent.click(screen.getByRole("button", { name: "Back to messages" }));
  expect(options.filters?.status).toEqual(["QUEUED"]);
  expect(screen.getByRole("status").textContent).toBe(
    "Observing message updates.",
  );
  for (const name of ["Send", "Reply", "Mark read", "Revive"])
    expect(screen.queryByRole("button", { name, exact: true })).toBeNull();
  feed = { ...feed, error: new MessageReadError("gap"), live: false };
  rerender(<MessageInbox origin="http://selected" />);
  expect(screen.getByRole("status").textContent).toContain("Retry");
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(refresh).toHaveBeenCalledTimes(1);
  feed = { ...feed, error: null, newData: true };
  rerender(<MessageInbox origin="http://selected" locale="zh-CN" />);
  expect(screen.getByRole("status").textContent).toContain("有新消息");
});
