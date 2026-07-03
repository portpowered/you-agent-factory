import { render, screen } from "@testing-library/react";

import {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";

describe("WidgetEmptyState", () => {
  it("renders compact dashboard empty states through the component contract", () => {
    render(
      <WidgetEmptyState compact>
        <h3>No trace selected</h3>
      </WidgetEmptyState>,
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
      <WidgetEmptyState>
        <WidgetEmptyStateTitle as="h2">
          No chart data
        </WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Run the factory to populate this trend.
        </WidgetEmptyStateText>
      </WidgetEmptyState>,
    );

    const title = screen.getByRole("heading", {
      level: 2,
      name: "No chart data",
    });
    const body = screen.getByText("Run the factory to populate this trend.");

    expect(title.className).toContain(WIDGET_FRAME_SECTION_HEADING_CLASS);
    expect(body.className).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
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
    expect(value.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
  });
});
