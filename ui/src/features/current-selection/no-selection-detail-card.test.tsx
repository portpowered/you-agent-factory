import { render, screen } from "@testing-library/react";
import { NoSelectionDetailCard } from "./no-selection-detail-card";

describe("NoSelectionDetailCard", () => {
  it("renders no-selection guidance in the same current selection card", () => {
    render(<NoSelectionDetailCard />);

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(
      screen.getByText("Select a workstation, work item, or state node to inspect live details."),
    ).toBeTruthy();
  });
});
