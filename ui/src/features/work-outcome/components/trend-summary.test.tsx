import { render, screen } from "@testing-library/react";

import { WIDGET_FRAME_SUBTITLE_CLASS } from "@you-agent-factory/components/recipes";

import { TrendSummaryGrid, TrendSummaryMetric } from "./trend-summary";

describe("TrendSummary", () => {
  it("renders trend metrics as a shared description-list surface composition", () => {
    render(
      <TrendSummaryGrid aria-label="Trend summary">
        <TrendSummaryMetric label="Failed in range" value={2} />
      </TrendSummaryGrid>,
    );

    const label = screen.getByText("Failed in range");
    const value = screen.getByText("2");
    const summary = label.closest("dl");
    const metric = label.closest("div");

    expect(summary.className).toContain("md:grid-cols-3");
    expect(metric?.className).toContain("border-outline");
    expect(metric?.className).toContain("bg-surface-container-low");
    expect(label.tagName).toBe("DT");
    expect(value.tagName).toBe("DD");
    expect(value.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
  });
});
