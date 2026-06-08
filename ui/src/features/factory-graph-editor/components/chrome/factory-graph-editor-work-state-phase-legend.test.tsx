import { render, screen } from "@testing-library/react";

import { FactoryGraphEditorWorkStatePhaseLegend } from "../chrome/factory-graph-editor-work-state-phase-legend";

describe("FactoryGraphEditorWorkStatePhaseLegend", () => {
  it("renders always-visible lifecycle swatches and localized labels", () => {
    render(<FactoryGraphEditorWorkStatePhaseLegend visible={true} />);

    const legend = screen.getByLabelText("Work state lifecycle colors");
    expect(
      legend.getAttribute("data-factory-graph-work-state-phase-legend"),
    ).toBe("");
    expect(screen.getByText("Initial")).toBeTruthy();
    expect(screen.getByText("Processing")).toBeTruthy();
    expect(screen.getByText("Completed")).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();

    const swatches = legend.querySelectorAll("[aria-hidden='true']");
    expect(swatches.length).toBe(4);
    expect(swatches[0]?.className).toContain("border-info-border");
    expect(swatches[1]?.className).toContain("border-af-warning-border");
    expect(swatches[2]?.className).toContain("border-af-success-border");
    expect(swatches[3]?.className).toContain("border-af-danger-border");
  });

  it("hides the legend when not visible", () => {
    render(<FactoryGraphEditorWorkStatePhaseLegend visible={false} />);

    expect(screen.queryByLabelText("Work state lifecycle colors")).toBeNull();
  });
});
