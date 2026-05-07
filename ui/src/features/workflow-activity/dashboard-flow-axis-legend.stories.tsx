import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../styles.css";
import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
} from "./dashboard-flow-axis-legend";
import { getDashboardFlowAxisLegendMessages } from "./messages/dashboard-flow-axis-legend";

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
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("en")}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("en")}
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getDashboardFlowAxisLegendMessages("en");
    const expandButton = canvas.getByRole("button", {
      name: messages.expandToggleLabel("graph legend"),
    });

    await expect(expandButton).toBeVisible();
    await expect(expandButton).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByLabelText(messages.title)).toBeNull();

    await userEvent.click(expandButton);

    const legend = canvas.getByLabelText(messages.title);
    const legendScope = within(legend);
    const collapseButton = canvas.getByRole("button", {
      name: messages.collapseToggleLabel("graph legend"),
    });

    await expect(legend).toBeVisible();
    await expect(collapseButton).toHaveAttribute("aria-expanded", "true");
    await expect(legendScope.getByText(messages.edgeLabels.activeFlow)).toBeVisible();
    await expect(legendScope.getByText(messages.edgeLabels.failurePath)).toBeVisible();
    for (const [label, kind] of [
      [messages.iconLabels.queue, "queue"],
      [messages.iconLabels.processing, "processing"],
      [messages.iconLabels.terminal, "terminal"],
      [messages.iconLabels.failed, "failed"],
      [messages.iconLabels.resource, "resource"],
      [messages.iconLabels.constraint, "constraint"],
      [messages.iconLabels.limit, "limit"],
      [messages.iconLabels.workstation, "workstation"],
      [messages.iconLabels.repeater, "repeater"],
      [messages.iconLabels.cron, "cron"],
      [messages.iconLabels["active-work"], "active-work"],
      [messages.iconLabels.exhaustion, "exhaustion"],
    ]) {
      await expect(legendScope.getByRole("img", { name: messages.iconLabel(label) })).toHaveAttribute(
        "data-graph-semantic-icon",
        kind,
      );
      await expect(legendScope.getByText(label)).toBeVisible();
    }

    await userEvent.click(collapseButton);

    await expect(canvas.queryByLabelText(messages.title)).toBeNull();
    await expect(
      canvas.getByRole("button", { name: messages.expandToggleLabel("graph legend") }),
    ).toBeVisible();
  },
};

export const Expanded = {
  render: () => (
    <LegendStoryFrame>
      <DashboardFlowAxisLegend
        className="relative left-0 top-0"
        defaultExpanded={true}
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("en")}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("en")}
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const legend = within(canvasElement).getByLabelText(
      getDashboardFlowAxisLegendMessages("en").title,
    );

    await expect(legend).toBeVisible();
  },
};

export const Narrow = {
  render: () => (
    <LegendStoryFrame className="relative w-[320px] min-h-[320px] rounded-3xl bg-af-bg p-4">
      <DashboardFlowAxisLegend
        className="relative left-0 top-0"
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("en")}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("en")}
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getDashboardFlowAxisLegendMessages("en");
    const expandButton = canvas.getByRole("button", {
      name: messages.expandToggleLabel("graph legend"),
    });

    await userEvent.click(expandButton);

    const legend = canvas.getByLabelText(messages.title);
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
    await expect(canvas.getByText(messages.iconLabels.exhaustion)).toBeVisible();
  },
};

export const Japanese = {
  render: () => (
    <LegendStoryFrame>
      <DashboardFlowAxisLegend
        className="relative left-0 top-0"
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("ja")}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("ja")}
        locale="ja"
      />
    </LegendStoryFrame>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const messages = getDashboardFlowAxisLegendMessages("ja");
    const expandButton = await canvas.findByRole("button", {
      name: messages.expandToggleLabel(messages.title),
    });

    await expect(expandButton).toBeVisible();
    await userEvent.click(expandButton);
    await expect(canvas.getByLabelText(messages.title)).toBeVisible();
    await expect(canvas.getByText(messages.iconLabels.queue)).toBeVisible();
  },
};
