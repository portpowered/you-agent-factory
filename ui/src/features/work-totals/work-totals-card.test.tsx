import { render, screen } from "@testing-library/react";
import { WorkTotalsCard } from "./work-totals-card";

describe("WorkTotalsCard", () => {
  it("renders work totals as a reusable bento card", () => {
    render(
      <WorkTotalsCard
        completedCount={3}
        dispatchedCount={5}
        failedCount={1}
        inFlightDispatchCount={2}
      />,
    );

    expect(screen.getByRole("heading", { name: "Work totals" })).toBeTruthy();
    expect(screen.getByText("In progress")).toBeTruthy();
    expect(screen.getByText("Dispatched")).toBeTruthy();
  });
});
