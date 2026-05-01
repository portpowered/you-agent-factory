import { fireEvent, render, screen, within } from "@testing-library/react";

import {
  DashboardFlowAxisLegend,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS,
} from "./dashboard-flow-axis-legend";

describe("DashboardFlowAxisLegend", () => {
  it("starts minimized and reveals the dashboard flow semantic vocabulary when expanded", () => {
    render(
      <DashboardFlowAxisLegend
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS}
      />,
    );

    const expandButton = screen.getByRole("button", { name: "Expand graph legend" });

    expect(expandButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByLabelText("Graph legend")).toBeNull();

    fireEvent.click(expandButton);

    const legend = screen.getByLabelText("Graph legend");
    const legendScope = within(legend);
    const collapseButton = screen.getByRole("button", { name: "Collapse graph legend" });
    const queueLabel = legend.querySelector("[data-legend-icon='queue'] span");
    const activeFlowLabel = legend.querySelector("[data-legend-edge='active-flow'] span:last-child");

    expect(legendScope.getByText("Active flow")).toBeTruthy();
    expect(legendScope.getByText("Failure path")).toBeTruthy();
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
      const icon = legendScope.getByRole("img", { name: `${label} legend icon` });

      expect(icon.getAttribute("data-graph-semantic-icon")).toBe(kind);
      expect(legendScope.getByText(label)).toBeTruthy();
      expect(legend.querySelector(`[data-legend-icon='${kind}']`)).toBeTruthy();
    }

    fireEvent.click(collapseButton);

    expect(screen.queryByLabelText("Graph legend")).toBeNull();
    expect(screen.getByRole("button", { name: "Expand graph legend" })).toBeTruthy();
  });

  it("allows callers to override the accessible label, initial state, and container classes", () => {
    render(
      <DashboardFlowAxisLegend
        ariaLabel="Current activity legend"
        defaultExpanded={true}
        className="relative top-0"
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS.slice(0, 1)}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS.slice(0, 2)}
      />,
    );

    const legendContainer = document.querySelector("[data-dashboard-flow-axis-legend]");
    const legend = screen.getByLabelText("Current activity legend");
    const collapseButton = screen.getByRole("button", {
      name: "Collapse current activity legend",
    });

    expect(legendContainer?.className).toContain("relative");
    expect(legendContainer?.className).toContain("top-0");
    expect(collapseButton.getAttribute("aria-expanded")).toBe("true");
    expect(within(legend).getByText("Active flow")).toBeTruthy();
    expect(within(legend).queryByText("Failure path")).toBeNull();
    expect(within(legend).getByText("Queue")).toBeTruthy();
    expect(within(legend).queryByText("Terminal")).toBeNull();
    expect(legend.className).toContain("dashboard-body-sm");
    expect(collapseButton.className).toContain("dashboard-eyebrow");
  });
});
