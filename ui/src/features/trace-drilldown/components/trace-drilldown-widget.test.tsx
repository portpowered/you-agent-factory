import { render, screen } from "@testing-library/react";

import { TraceDrilldownWidget } from "./trace-drilldown-widget";

describe("TraceDrilldownWidget", () => {
  it("keeps the compact trace shell inside the card while preserving a readable minimum height", () => {
    render(
      <TraceDrilldownWidget
        state={{ message: "Select work to inspect trace history.", status: "idle" }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expect(card.className).toContain("h-full");
    expect(card.className).toContain("min-h-[24rem]");
  });
});
