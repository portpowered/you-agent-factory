import { cleanup, render, screen, within } from "@testing-library/react";

import type { DashboardTrace } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { TraceGridBentoCard } from "./trace-grid-card";

const longDispatchTrace: DashboardTrace = {
  dispatches: Array.from({ length: 10 }, (_, index) => ({
    current_chaining_trace_id: `trace-chain-${index}`,
    dispatch_id: `dispatch-long-${index}`,
    duration_millis: 1000 + index,
    end_time: `2026-04-08T12:00:${String(index + 1).padStart(2, "0")}Z`,
    outcome: "ACCEPTED" as const,
    start_time: `2026-04-08T12:00:${String(index).padStart(2, "0")}Z`,
    transition_id: `transition-${index}`,
    workstation_name: `Workstation ${index}`,
  })),
  trace_id: "trace-long-dispatch",
  transition_ids: [],
  work_ids: [],
  workstation_sequence: Array.from(
    { length: 10 },
    (_, index) => `Workstation ${index}`,
  ),
};

function expectNoVerticalScrollContainer(element: Element) {
  expect(element.className).toContain("overflow-y-clip");
  expect(element.className).not.toMatch(/overflow-y-(auto|scroll)/);
  const style = window.getComputedStyle(element);
  expect(style.overflowY).not.toBe("auto");
  expect(style.overflowY).not.toBe("scroll");
}

describe("TraceGridBentoCard dispatch table height", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders every dispatch row without a nested vertical scroll container", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const tableRegion = card.querySelector("[data-trace-dispatch-table]");
    if (!tableRegion) {
      throw new Error("Expected trace dispatch table region to render.");
    }

    expectNoVerticalScrollContainer(tableRegion);
    expect(within(card).getAllByRole("row")).toHaveLength(11);
    for (let index = 0; index < 10; index += 1) {
      expect(within(card).getByText(`dispatch-long-${index}`)).toBeTruthy();
    }
  });
});
