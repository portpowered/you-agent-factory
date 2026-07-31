import { render, screen, within } from "@testing-library/react";

import { getBentoGridItemHeightPx } from "../../bento/components/agent-bento";
import { expectNoVerticalScrollContainer } from "../../trace-drilldown/lib/trace-grid-card-scroll-test-helpers";
import { WorkTotalsCard } from "./work-totals-card";

const REPRESENTATIVE_TOTALS = {
  completedCount: 3,
  dispatchedCount: 5,
  failedCount: 1,
  inFlightDispatchCount: 2,
} as const;

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

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
    const inProgressCard = screen.getByLabelText("In progress: 2");
    const completedCard = screen.getByLabelText("Completed: 3");
    const failedCard = screen.getByLabelText("Failed: 1");
    const dispatchedCard = screen.getByLabelText("Dispatched: 5");
    const cardShell = screen.getByRole("article", { name: "Work totals" });
    const cardHeader = cardShell.querySelector("header");

    expect(screen.getByRole("heading", { name: "Work totals" })).toBeTruthy();
    expect(workTotals.className).toContain("grid-cols-4");
    expect(cardHeader?.className).toContain("min-h-11");
    expect(cardHeader?.className).toContain("px-3");
    expect(cardHeader?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(cardHeader?.className).toContain("cursor-grab");
    expect(cardShell.dataset.dashboardPanelShell).toBe("grid-card");
    expect(cardShell.className).toContain("shadow-af-card");
    expect(
      within(cardShell).queryByRole("button", { name: "Move Work totals" }),
    ).toBeNull();
    expect(
      within(
        requireValue(cardHeader, "expected work totals header"),
      ).queryAllByRole("button", { hidden: true }),
    ).toHaveLength(0);
    expect(
      cardHeader?.querySelector(
        '[aria-expanded="true"], [aria-expanded="false"]',
      ),
    ).toBeNull();
    expect(screen.getByLabelText("In progress: 2")).toBeTruthy();
    expect(screen.getByLabelText("Completed: 3")).toBeTruthy();
    expect(screen.getByLabelText("Failed: 1")).toBeTruthy();
    expect(screen.getByLabelText("Dispatched: 5")).toBeTruthy();
    expect(inProgressCard.className).toContain("border-af-info-border");
    expect(inProgressCard.className).toContain("bg-info-container");
    expect(completedCard.className).toContain("border-af-success-border");
    expect(completedCard.className).toContain("bg-success-container");
    expect(failedCard.className).toContain("border-af-danger-border");
    expect(failedCard.className).toContain("bg-error-container");
    expect(dispatchedCard.className).toContain("border-af-info-border");
    expect(dispatchedCard.className).toContain("bg-info-container");
    expect(dispatchedCard.className).not.toContain("border-af-success-border");
    expect(dispatchedCard.className).not.toContain("border-af-danger-border");
  });

  it("fits default bento height without vertical scroll for representative counts", () => {
    const defaultBentoHeightPx = getBentoGridItemHeightPx(2);

    render(
      <div style={{ height: defaultBentoHeightPx, width: 360 }}>
        <WorkTotalsCard {...REPRESENTATIVE_TOTALS} />
      </div>,
    );

    const card = screen.getByRole("article", { name: "Work totals" });
    const header = card.querySelector("header");
    const body = header?.nextElementSibling;

    expect(card.querySelector("[data-radix-scroll-area-viewport]")).toBeNull();
    expect(body).toBeTruthy();
    if (!(body instanceof HTMLElement)) {
      throw new Error("Expected work totals card body.");
    }

    expect(body.className).toContain("!h-auto");
    expect(body.className).toContain("!pb-2.5");
    expect(body.className).not.toMatch(/overflow-y-(auto|scroll)/);
    expectNoVerticalScrollContainer(body);

    const statCard = screen.getByLabelText("In progress: 2");
    expect(statCard.className).toContain("p-2");
    expect(within(statCard).getByText("2").className).toContain(
      "text-[1.2rem]",
    );
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

describe("WorkTotalsCard header layout", () => {
  it("keeps the compact header action in the title row", () => {
    render(
      <WorkTotalsCard
        {...REPRESENTATIVE_TOTALS}
        headerAction={<button type="button">Remove work totals</button>}
      />,
    );

    const card = screen.getByRole("article", { name: "Work totals" });
    const header = requireValue(
      card.querySelector("header"),
      "Expected work totals header.",
    );
    const tools = header.lastElementChild;
    const removeButton = within(card).getByRole("button", {
      name: "Remove work totals",
    });

    expect(header.className).not.toContain("flex-wrap");
    expect(tools?.className).toContain("ml-auto");
    expect(tools?.className).not.toContain("w-full");
    expect(tools?.className).not.toContain("flex-wrap");
    expect(tools?.contains(removeButton)).toBe(true);
  });
});
