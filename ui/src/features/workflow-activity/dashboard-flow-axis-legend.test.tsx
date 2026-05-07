import { fireEvent, render, screen, within } from "@testing-library/react";

import {
  DashboardFlowAxisLegend,
  getDefaultDashboardFlowAxisLegendEdgeItems,
  getDefaultDashboardFlowAxisLegendIconItems,
} from "./dashboard-flow-axis-legend";
import { getDashboardFlowAxisLegendMessages } from "./messages/dashboard-flow-axis-legend";

describe("DashboardFlowAxisLegend", () => {
  it("starts minimized and reveals the dashboard flow semantic vocabulary when expanded", () => {
    const messages = getDashboardFlowAxisLegendMessages("en");

    render(
      <DashboardFlowAxisLegend
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("en")}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("en")}
      />,
    );

    const expandButton = screen.getByRole("button", {
      name: messages.expandToggleLabel("graph legend"),
    });

    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText(messages.title)).toBeNull();

    fireEvent.click(expandButton);

    const legend = screen.getByLabelText(messages.title);
    const legendScope = within(legend);
    const collapseButton = screen.getByRole("button", {
      name: messages.collapseToggleLabel("graph legend"),
    });
    const queueLabel = legend.querySelector("[data-legend-icon='queue'] span");
    const activeFlowLabel = legend.querySelector("[data-legend-edge='active-flow'] span:last-child");

    expect(legendScope.getByText(messages.edgeLabels.activeFlow)).toBeTruthy();
    expect(legendScope.getByText(messages.edgeLabels.failurePath)).toBeTruthy();
    expect(legend.querySelector("[data-legend-edge='active-flow']")).toBeTruthy();
    expect(legend.querySelector("[data-legend-edge='failure-path']")).toBeTruthy();
    expect(legend.querySelector("[data-legend-flow]")).toBeTruthy();
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    expect(legend.className).toContain("dashboard-body-sm");
    expect(legend.className).not.toContain("text-[0.72rem]");
    expect(expandButton.className).toContain("dashboard-eyebrow");
    expect(collapseButton.className).toContain("dashboard-eyebrow");
    expect(activeFlowLabel?.className).toContain("dashboard-body-sm");
    expect(queueLabel?.className).toContain("dashboard-body-sm");
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
      const icon = legendScope.getByRole("img", { name: messages.iconLabel(label) });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(kind);
      expect(legendScope.getByText(label)).toBeTruthy();
      expect(legend.querySelector(`[data-legend-icon='${kind}']`)).toBeTruthy();
    }

    fireEvent.click(collapseButton);

    expect(screen.queryByLabelText(messages.title)).toBeNull();
    expect(
      screen.getByRole("button", { name: messages.expandToggleLabel("graph legend") }),
    ).toBeTruthy();
  });

  it("allows callers to override the accessible label, initial state, and container classes", () => {
    const messages = getDashboardFlowAxisLegendMessages("en");

    render(
      <DashboardFlowAxisLegend
        ariaLabel="Current activity legend"
        defaultExpanded={true}
        className="relative top-0"
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("en").slice(0, 1)}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("en").slice(0, 2)}
      />,
    );

    const legendContainer = document.querySelector("[data-dashboard-flow-axis-legend]");
    const legend = screen.getByLabelText("Current activity legend");
    const collapseButton = screen.getByRole("button", {
      name: messages.collapseToggleLabel("current activity legend"),
    });

    expect(legendContainer?.className).toContain("relative");
    expect(legendContainer?.className).toContain("top-0");
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    expect(within(legend).getByText(messages.edgeLabels.activeFlow)).toBeTruthy();
    expect(within(legend).queryByText(messages.edgeLabels.failurePath)).toBeNull();
    expect(within(legend).getByText(messages.iconLabels.queue)).toBeTruthy();
    expect(within(legend).queryByText(messages.iconLabels.terminal)).toBeNull();
    expect(legend.className).toContain("dashboard-body-sm");
    expect(collapseButton.className).toContain("dashboard-eyebrow");
  });

  it("renders localized title, toggle copy, and icon accessibility labels from the workflow-activity catalog", () => {
    const messages = getDashboardFlowAxisLegendMessages("ja");

    render(
      <DashboardFlowAxisLegend
        edgeItems={getDefaultDashboardFlowAxisLegendEdgeItems("ja")}
        iconItems={getDefaultDashboardFlowAxisLegendIconItems("ja")}
        locale="ja"
      />,
    );

    const expandButton = screen.getByRole("button", {
      name: messages.expandToggleLabel(messages.title),
    });

    expect(screen.getByText(messages.minimizedLabel)).toBeTruthy();

    fireEvent.click(expandButton);

    const legend = screen.getByLabelText(messages.title);

    expect(within(legend).getByText(messages.edgeLabels.activeFlow)).toBeTruthy();
    expect(
      within(legend).getByRole("img", {
        name: messages.iconLabel(messages.iconLabels.queue),
      }),
    ).toBeTruthy();
  });
});
