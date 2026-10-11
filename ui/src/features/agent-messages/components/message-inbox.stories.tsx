import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { AgentMessage } from "../../../api/agent-messages/types";
import { matchesMessageFilters } from "../lib/message-page";
import { messageFixture } from "../lib/testing/message-fixture";
import { MessageInbox } from "./message-inbox";

const meta = {
  title: "Agent Messages/Read-only inbox",
  component: MessageInbox,
  parameters: { layout: "padded" },
} satisfies Meta<typeof MessageInbox>;
export default meta;
type Story = StoryObj<typeof meta>;

const rows: AgentMessage[] = [
  messageFixture({
    body: "Can the requester continue?",
    reason: "REVIVE_PENDING",
  }),
  messageFixture({
    messageId: "reply",
    inReplyTo: "msg-question",
    body: "Continue with the approved change.",
    status: "DELIVERED_REVIVE",
    from: {
      principal: "WORKER",
      workerSessionId: "lead",
      factorySessionId: "factory-b",
    },
    to: {
      kind: "WORKER_SESSION",
      workerSessionId: "worker",
      factorySessionId: "factory-a",
      deliveredWorkerSessionId: "successor",
    },
    revivedWorkerSessionId: "successor",
  }),
];

export const HoldReplyAndRevival: Story = {
  args: {
    origin: "http://story-host",
    list: async (filters) => ({
      messages: rows.filter((row) => matchesMessageFilters(row, filters)),
    }),
    follow: async (options) =>
      new Promise((resolve) =>
        options.signal?.addEventListener("abort", () => resolve(), {
          once: true,
        }),
      ),
    readIdentity: async (options) =>
      options.workerSessionId === "worker"
        ? {
            workerSessionId: "worker",
            factorySessionId: "factory-a",
            successorWorkerSessionId: "successor",
          }
        : {
            workerSessionId: "successor",
            predecessorWorkerSessionId: "worker",
            direct: true,
          },
    onNavigate: () => {},
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("Can the requester continue?"),
    ).toBeVisible();
    await expect(canvas.getByText("Revival pending")).toBeVisible();
    await expect(canvas.getByText("Delivered by revival")).toBeVisible();
    await userEvent.click(canvas.getByRole("combobox", { name: "Status" }));
    await userEvent.click(
      within(canvasElement.ownerDocument.body).getByRole("option", {
        name: "Queued",
        exact: true,
      }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: "Apply filters" }),
    );
    await expect(
      canvas.queryByText("Continue with the approved change."),
    ).toBeNull();
    await userEvent.click(
      canvas.getByRole("button", { name: "Open thread msg-question" }),
    );
    await expect(
      await canvas.findByText("Continue with the approved change."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", {
        name: "Open revived Worker Session successor",
      }),
    ).toBeVisible();
  },
};
