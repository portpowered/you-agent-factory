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
    const expectedSwatches = [
      "border-info-border bg-info-container",
      "border-af-warning-border bg-warning-container",
      "border-af-success-border bg-success-container",
      "border-af-danger-border bg-error-container",
    ];
    for (const [index, expectedClass] of expectedSwatches.entries()) {
      expect(swatches[index]?.className).toContain(expectedClass);
    }
  });

  it("hides the legend when not visible", () => {
    render(<FactoryGraphEditorWorkStatePhaseLegend visible={false} />);

    expect(screen.queryByLabelText("Work state lifecycle colors")).toBeNull();
  });
});
