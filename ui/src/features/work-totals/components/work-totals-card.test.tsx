import { render, screen, within } from "@testing-library/react";
import { WorkTotalsCard } from "./work-totals-card";

describe("WorkTotalsCard", () => {
  it("renders localized totals with semantic status borders and a neutral dispatched card", () => {
    render(
      <WorkTotalsCard
        completedCount={3}
        dispatchedCount={5}
        failedCount={1}
        inFlightDispatchCount={2}
      />,
    );

    const workTotals = screen.getByLabelText("work totals");
    const inProgressCard = within(workTotals)
      .getByText("In progress")
      .closest("article");
    const completedCard = within(workTotals)
      .getByText("Completed")
      .closest("article");
    const failedCard = within(workTotals)
      .getByText("Failed")
      .closest("article");
    const dispatchedCard = within(workTotals)
      .getByText("Dispatched")
      .closest("article");
    const cardShell = screen.getByRole("article", { name: "Work totals" });
    const cardHeader = cardShell.querySelector("header");
    const moveHandle = within(cardShell).getByRole("button", {
      name: "Move Work totals",
    });

    expect(screen.getByRole("heading", { name: "Work totals" })).toBeTruthy();
    expect(workTotals.className).toContain("grid-cols-4");
    expect(cardHeader?.className).toContain("min-h-11");
    expect(cardHeader?.className).toContain("px-3");
    expect(moveHandle.className).toContain("h-10");
    expect(moveHandle.className).toContain("w-10");
    expect(screen.getByLabelText("In progress: 2")).toBeTruthy();
    expect(screen.getByLabelText("Completed: 3")).toBeTruthy();
    expect(screen.getByLabelText("Failed: 1")).toBeTruthy();
    expect(screen.getByLabelText("Dispatched: 5")).toBeTruthy();
    expect(inProgressCard?.className).toContain("border-af-info-border");
    expect(inProgressCard?.className).toContain("bg-af-info-surface");
    expect(completedCard?.className).toContain("border-af-success-border");
    expect(completedCard?.className).toContain("bg-af-success-surface");
    expect(failedCard?.className).toContain("border-af-danger-border");
    expect(failedCard?.className).toContain("bg-af-danger-surface");
    expect(dispatchedCard?.className).toContain("border-af-border");
    expect(dispatchedCard?.className).not.toContain("border-af-info-border");
    expect(dispatchedCard?.className).not.toContain("border-af-success-border");
    expect(dispatchedCard?.className).not.toContain("border-af-danger-border");
  });

  it("renders zh-CN widget labels and accessible stat values", () => {
    render(
      <WorkTotalsCard
        completedCount={3}
        dispatchedCount={5}
        failedCount={1}
        inFlightDispatchCount={2}
        locale="zh-CN"
      />,
    );

    expect(screen.getByRole("heading", { name: "工作总计" })).toBeTruthy();
    expect(screen.getByText("进行中")).toBeTruthy();
    expect(screen.getByText("已分派")).toBeTruthy();
    expect(screen.getByLabelText("已完成：3")).toBeTruthy();
    expect(screen.getByLabelText("进行中：2")).toBeTruthy();
    expect(screen.getByLabelText("失败：1")).toBeTruthy();
    expect(screen.getByLabelText("已分派：5")).toBeTruthy();
  });
});
