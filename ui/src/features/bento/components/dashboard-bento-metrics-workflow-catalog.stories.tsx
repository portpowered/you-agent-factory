import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import { WorkTotalsWidget } from "../../work-totals/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";
import {
  emptyDashboardSnapshot,
  expectBentoHeaderDragSurface,
  layoutFor,
  renderCardFrame,
  semanticWorkflowDashboardSnapshot,
  WorkflowGraphCardStory,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const WorkTotals = {
  render: () =>
    renderCardFrame({
      children: (
        <WorkTotalsWidget snapshot={semanticWorkflowDashboardSnapshot} />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.workTotals, {
        h: 2,
        id: "work-totals::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Work totals" });

    await expect(within(card).getByText("Completed")).toBeVisible();
    expectBentoHeaderDragSurface(card, "Work totals");
  },
};

export const WorkTotalsEmpty = {
  render: () =>
    renderCardFrame({
      children: <WorkTotalsWidget snapshot={emptyDashboardSnapshot} />,
      layout: layoutFor(DASHBOARD_WIDGET_IDS.workTotals, {
        h: 2,
        id: "work-totals-empty::story",
        w: 6,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Work totals" });

    await expect(within(card).getByLabelText("Completed: 0")).toBeVisible();
    await expect(within(card).getByLabelText("Failed: 0")).toBeVisible();
  },
};

export const WorkflowGraph = {
  render: () => <WorkflowGraphCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Factory graph" });

    await expect(
      await within(card).findByRole("button", {
        name: "Select Implement workstation",
      }),
    ).toBeVisible();
    await userEvent.click(
      await within(card).findByRole("button", {
        name: "Select Implement workstation",
      }),
    );
    await expect(
      await within(card).findByRole("button", {
        name: "Select Implement workstation",
      }),
    ).toHaveAttribute("aria-pressed", "true");
    expectBentoHeaderDragSurface(card, "Factory graph");
  },
};
