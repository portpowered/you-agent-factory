import { render, screen } from "@testing-library/react";

import { CurrentSelectionHistoryCard } from "./current-selection-history-card";

describe("CurrentSelectionHistoryCard", () => {
  it("keeps history cards on the subtle surface", () => {
    render(<CurrentSelectionHistoryCard>row</CurrentSelectionHistoryCard>);

    const card = screen.getByText("row");

    expect(card.className).toContain("bg-surface-container-low");
    expect(card.className).not.toContain("bg-surface-container-high");
  });
});
