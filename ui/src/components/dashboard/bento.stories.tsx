import { expect, userEvent, within } from "storybook/test";

import "../../styles.css";
import { AgentBentoCard, AgentBentoLayout, type AgentBentoLayoutItem } from "./bento";

const defaultLayout: AgentBentoLayoutItem[] = [{ id: "summary", x: 0, y: 0, w: 4, h: 2 }];

const multiCardLayout: AgentBentoLayoutItem[] = [
  { id: "activity", x: 0, y: 0, w: 5, h: 3 },
  { id: "trace", x: 5, y: 0, w: 4, h: 3 },
  { id: "terminal", x: 9, y: 0, w: 3, h: 3 },
];

function card(id: string, title: string, body: string) {
  return {
    id,
    children: (
      <AgentBentoCard title={title}>
        <p>{body}</p>
      </AgentBentoCard>
    ),
  };
}

export default {
  title: "Agent Factory/Bento Layout",
  component: AgentBentoLayout,
};

export const Default = {
  render: () => (
    <div style={{ padding: "1rem" }}>
      <AgentBentoLayout
        cards={[card("summary", "Factory summary", "One bento card can hold plain text.")]}
        layout={defaultLayout}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("region", { name: "Agent Factory bento board" }),
    ).toBeVisible();
    await expect(await canvas.findByRole("article", { name: "Factory summary" })).toBeVisible();
    await expect(await canvas.findByText("One bento card can hold plain text.")).toBeVisible();
  },
};

export const MultiCard = {
  render: () => (
    <div style={{ padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          card("activity", "Current activity", "Workflow graph card placeholder."),
          card("trace", "Trace grid", "Trace rows remain independently testable."),
          card("terminal", "Terminal work", "Completed and failed work share the board."),
        ]}
        layout={multiCardLayout}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const activity = await canvas.findByRole("article", { name: "Current activity" });
    const handle = within(activity).getByRole("button", { name: "Move Current activity" });

    await userEvent.pointer([
      { keys: "[MouseLeft>]", target: handle, coords: { x: 120, y: 40 } },
      { target: handle, coords: { x: 280, y: 42 } },
      { keys: "[/MouseLeft]", target: handle, coords: { x: 280, y: 42 } },
    ]);

    await expect(activity).toBeVisible();
    await expect(await canvas.findByRole("article", { name: "Trace grid" })).toBeVisible();
    await expect(await canvas.findByRole("article", { name: "Terminal work" })).toBeVisible();
  },
};

export const ConstrainedWidth = {
  render: () => (
    <div style={{ maxWidth: "520px", padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          card("activity", "Current activity", "The layout can render in a narrow shell."),
          card("trace", "Trace grid", "Cards keep their content on the board."),
        ]}
        initialWidth={520}
        layout={[
          { id: "activity", x: 0, y: 0, w: 6, h: 2 },
          { id: "trace", x: 0, y: 2, w: 6, h: 2 },
        ]}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(await canvas.findByText("The layout can render in a narrow shell.")).toBeVisible();
    await expect(await canvas.findByText("Cards keep their content on the board.")).toBeVisible();
  },
};
