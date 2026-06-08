import { render, screen } from "@testing-library/react";

import {
  DashboardEmptyState,
  DashboardEmptyStateText,
  DashboardEmptyStateTitle,
  WidgetSubtitle,
} from "./widget-frame";

describe("DashboardEmptyState", () => {
  it("renders compact dashboard empty states through the component contract", () => {
    render(
      <DashboardEmptyState compact>
        <h3>No trace selected</h3>
      </DashboardEmptyState>,
    );

    const emptyHeading = screen.getByRole("heading", {
      name: "No trace selected",
    });
    expect(emptyHeading.parentElement?.className).toContain("min-h-0");
    expect(emptyHeading.parentElement?.className).toContain(
      "bg-surface-container-low",
    );
  });

  it("renders empty-state title and body copy through shared typography roles", () => {
    render(
      <DashboardEmptyState>
        <DashboardEmptyStateTitle as="h2">
          No chart data
        </DashboardEmptyStateTitle>
        <DashboardEmptyStateText>
          Run the factory to populate this trend.
        </DashboardEmptyStateText>
      </DashboardEmptyState>,
    );

    const title = screen.getByRole("heading", {
      level: 2,
      name: "No chart data",
    });
    const body = screen.getByText("Run the factory to populate this trend.");

    expect(title.className).toContain("af-dashboard-section-heading");
    expect(body.className).toContain("af-dashboard-body-text");
    expect(body.className).toContain("m-0");
  });
});

describe("WidgetSubtitle", () => {
  it("supports subtitle text on non-paragraph semantic elements", () => {
    render(
      <dl>
        <WidgetSubtitle as="dd">42 completed</WidgetSubtitle>
      </dl>,
    );

    const value = screen.getByText("42 completed");

    expect(value.tagName).toBe("DD");
    expect(value.className).toContain("af-dashboard-widget-subtitle");
  });
});
