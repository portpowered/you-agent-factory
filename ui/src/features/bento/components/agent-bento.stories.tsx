import { expect, userEvent, within } from "storybook/test";

import { NoSelectionDetailCard } from "../../current-selection/components/no-selection-detail-card";
import { WorkTotalsCard } from "../../work-totals/public";
import "../../../styles.css";
import {
  AgentBentoCard,
  AgentBentoLayout,
  type AgentBentoLayoutItem,
} from "./agent-bento";

const defaultLayout: AgentBentoLayoutItem[] = [
  { h: 2, id: "summary", widgetType: "summary", w: 4, x: 0, y: 0 },
];

const multiCardLayout: AgentBentoLayoutItem[] = [
  { h: 3, id: "activity", widgetType: "activity", w: 5, x: 0, y: 0 },
  { h: 3, id: "trace", widgetType: "trace", w: 4, x: 5, y: 0 },
  { h: 3, id: "terminal", widgetType: "terminal", w: 3, x: 9, y: 0 },
];

const featureBoardLayout: AgentBentoLayoutItem[] = [
  { h: 2, id: "work-totals", widgetType: "work-totals", w: 4, x: 0, y: 0 },
  { h: 4, id: "current-selection", widgetType: "current-selection", w: 8, x: 4, y: 0 },
];

function card(id: string, title: string, body: string) {
  return {
    id,
    widgetType: id,
    children: (
      <AgentBentoCard title={title}>
        <p>{body}</p>
      </AgentBentoCard>
    ),
  };
}

export default {
  title: "you-agent-factory/Bento Layout",
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
      await canvas.findByRole("region", { name: "you-agent-factory bento board" }),
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

export const RealDashboardState = {
  render: () => (
    <div style={{ padding: "1rem" }}>
      <AgentBentoLayout
        cards={[
          {
            id: "work-totals",
            widgetType: "work-totals",
            children: (
              <WorkTotalsCard
                completedCount={3}
                dispatchedCount={5}
                failedCount={1}
                inFlightDispatchCount={2}
              />
            ),
          },
          {
            id: "current-selection",
            widgetType: "current-selection",
            children: <NoSelectionDetailCard />,
          },
        ]}
        initialWidth={1180}
        layout={featureBoardLayout}
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(await canvas.findByRole("article", { name: "Work totals" })).toBeVisible();
    await expect(await canvas.findByText("In progress")).toBeVisible();
    await expect(await canvas.findByRole("article", { name: "Current selection" })).toBeVisible();
    await expect(await canvas.findByRole("button", { name: "Undo selection" })).toBeDisabled();
    await expect(
      await canvas.findByText(
        "Select a workstation, work item, or state node to inspect live details.",
      ),
    ).toBeVisible();
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
          { h: 2, id: "activity", widgetType: "activity", w: 6, x: 0, y: 0 },
          { h: 2, id: "trace", widgetType: "trace", w: 6, x: 0, y: 2 },
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
