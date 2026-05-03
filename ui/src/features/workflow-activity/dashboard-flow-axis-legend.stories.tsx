import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../styles.css";
import {
  DashboardFlowAxisLegend,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS,
} from "./dashboard-flow-axis-legend";

export default {
  title: "Agent Factory/Dashboard/Dashboard Flow Axis Legend",
  component: DashboardFlowAxisLegend,
  tags: ["test"],
};

function LegendStoryFrame({
  children,
  className = "relative min-h-[320px] rounded-3xl bg-af-bg p-8",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={className} data-dashboard-flow-axis-legend-story-frame="">
      {children}
    </div>
  );
}

export const Interactive = {
  render: () => (
    <LegendStoryFrame>
      <DashboardFlowAxisLegend
        className="relative left-0 top-0"
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS}
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const expandButton = canvas.getByRole("button", { name: "Expand graph legend" });

    await expect(expandButton).toBeVisible();
    await expect(expandButton).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByLabelText("Graph legend")).toBeNull();

    await userEvent.click(expandButton);

    const legend = canvas.getByLabelText("Graph legend");
    const legendScope = within(legend);
    const collapseButton = canvas.getByRole("button", { name: "Collapse graph legend" });

    await expect(legend).toBeVisible();
    await expect(collapseButton).toHaveAttribute("aria-expanded", "true");
    await expect(legendScope.getByText("Active flow")).toBeVisible();
    await expect(legendScope.getByText("Failure path")).toBeVisible();
    for (const [label, kind] of [
      ["Queue", "queue"],
      ["Processing", "processing"],
      ["Terminal", "terminal"],
      ["Failed state", "failed"],
      ["Resource", "resource"],
      ["Constraint", "constraint"],
      ["Limit", "limit"],
      ["Standard workstation", "workstation"],
      ["Repeater workstation", "repeater"],
      ["Cron workstation", "cron"],
      ["Active work", "active-work"],
      ["Exhaustion rule", "exhaustion"],
    ]) {
      await expect(legendScope.getByRole("img", { name: `${label} legend icon` })).toHaveAttribute(
        "data-graph-semantic-icon",
        kind,
      );
      await expect(legendScope.getByText(label)).toBeVisible();
    }

    await userEvent.click(collapseButton);

    await expect(canvas.queryByLabelText("Graph legend")).toBeNull();
    await expect(canvas.getByRole("button", { name: "Expand graph legend" })).toBeVisible();
  },
};

export const Expanded = {
  render: () => (
    <LegendStoryFrame>
      <DashboardFlowAxisLegend
        className="relative left-0 top-0"
        defaultExpanded={true}
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS}
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const legend = within(canvasElement).getByLabelText("Graph legend");

    await expect(legend).toBeVisible();
  },
};

export const Narrow = {
  render: () => (
    <LegendStoryFrame className="relative w-[320px] min-h-[320px] rounded-3xl bg-af-bg p-4">
      <DashboardFlowAxisLegend
        className="relative left-0 top-0"
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS}
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const expandButton = canvas.getByRole("button", { name: "Expand graph legend" });

    await userEvent.click(expandButton);

    const legend = canvas.getByLabelText("Graph legend");
    const storyFrame = canvasElement.querySelector<HTMLElement>(
      "[data-dashboard-flow-axis-legend-story-frame]",
    );
    const legendRect = legend.getBoundingClientRect();
    const storyFrameRect = storyFrame?.getBoundingClientRect();

    await expect(legend).toBeVisible();
    expect(storyFrame).toBeTruthy();
    expect(legend.className).toContain("dashboard-body-sm");
    expect(legendRect.left).toBeGreaterThanOrEqual((storyFrameRect?.left ?? 0) - 1);
    expect(legendRect.right).toBeLessThanOrEqual((storyFrameRect?.right ?? 0) + 1);
    await expect(canvas.getByText("Exhaustion rule")).toBeVisible();
  },
};

