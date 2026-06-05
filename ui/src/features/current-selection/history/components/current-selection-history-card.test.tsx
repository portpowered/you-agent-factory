import { render, screen } from "@testing-library/react";

import { CurrentSelectionHistoryCard } from "./current-selection-history-card";

describe("CurrentSelectionHistoryCard", () => {
  it("renders row content inside the low history surface", () => {
    render(<CurrentSelectionHistoryCard>row</CurrentSelectionHistoryCard>);

    expect(screen.getByText("row").closest("article")?.className).toContain(
      "bg-surface-container-low",
    );
  });
});
