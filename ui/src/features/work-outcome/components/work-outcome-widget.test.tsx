import { render, screen } from "@testing-library/react";

import type { WorkChartModel } from "../lib/trends";
import { WorkOutcomeWidget } from "./work-outcome-widget";

const emptyTrend: WorkChartModel = {
  delta: {
    completed: 0,
    failed: 0,
    inFlight: 0,
    queued: 0,
  },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

describe("WorkOutcomeWidget", () => {
  it("lets the chart card inherit the dashboard grid height instead of imposing an oversized shell minimum", () => {
    render(<WorkOutcomeWidget model={emptyTrend} />);

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expect(card.className).toContain("h-full");
    expect(card.className).toContain("min-h-0");
    expect(card.className).not.toContain("min-h-72");
  });
});
