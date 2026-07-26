import "../../../../testing/vitest-dom-capabilities.setup";

import { render, screen, within } from "@testing-library/react";

import type { FactoryGraphBulkSelectionSummary } from "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import { GraphBulkSelectionDetailCard } from "./graph-bulk-selection-detail-card";

const mixedSelectionSummary: FactoryGraphBulkSelectionSummary = {
  totalCount: 4,
  kindCounts: [
    { kind: "workstation", count: 1 },
    { kind: "worker", count: 1 },
    { kind: "edge", count: 2 },
  ],
};

describe("GraphBulkSelectionDetailCard", () => {
  it("renders total and per-kind counts without editable detail fields", () => {
    render(<GraphBulkSelectionDetailCard summary={mixedSelectionSummary} />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    expect(
      within(panel).getByText("Multiple graph items selected"),
    ).toBeTruthy();
    expect(within(panel).getByText("Selected items")).toBeTruthy();
    expect(within(panel).getByText("4")).toBeTruthy();
    expect(within(panel).getByText("Workstations")).toBeTruthy();
    expect(within(panel).getByText("Workers")).toBeTruthy();
    expect(within(panel).getByText("Edges")).toBeTruthy();
    expect(within(panel).queryByRole("textbox", { name: "Model" })).toBeNull();
    expect(within(panel).queryByRole("button", { name: "Save" })).toBeNull();
  });
});
