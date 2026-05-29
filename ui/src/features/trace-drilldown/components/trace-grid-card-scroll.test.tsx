import { cleanup, render, screen, within } from "@testing-library/react";

import type { DashboardTrace } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { TraceGridBentoCard } from "./trace-grid-card";

const longDispatchTrace: DashboardTrace = {
  dispatches: Array.from({ length: 12 }, (_, index) => ({
    current_chaining_trace_id: `trace-chain-${index}`,
    dispatch_id: `dispatch-scroll-${index}`,
    duration_millis: 1000 + index,
    end_time: `2026-04-08T12:00:${String(index + 1).padStart(2, "0")}Z`,
    outcome: "ACCEPTED" as const,
    start_time: `2026-04-08T12:00:${String(index).padStart(2, "0")}Z`,
    transition_id: `transition-${index}`,
    workstation_name: `Workstation ${index}`,
  })),
  trace_id: "trace-scroll-card",
  transition_ids: [],
  work_ids: [],
  workstation_sequence: Array.from(
    { length: 12 },
    (_, index) => `Workstation ${index}`,
  ),
};

function expectNoVerticalScrollContainer(element: Element) {
  expect(element.className).not.toMatch(/overflow-y-(auto|scroll)/);
  const style = window.getComputedStyle(element);
  expect(style.overflowY).not.toBe("auto");
  expect(style.overflowY).not.toBe("scroll");
}

function findTraceCardScrollContainer(card: HTMLElement): HTMLElement {
  const scrollContainer = card.querySelector("[data-trace-card-scroll]");
  if (!(scrollContainer instanceof HTMLElement)) {
    throw new Error("Expected trace card scroll container.");
  }

  return scrollContainer;
}

function constrainTraceCardHeight(card: HTMLElement, heightPx: number) {
  Object.defineProperty(card, "clientHeight", {
    configurable: true,
    value: heightPx,
  });
  const scrollContainer = findTraceCardScrollContainer(card);
  Object.defineProperty(scrollContainer, "clientHeight", {
    configurable: true,
    value: heightPx - 64,
  });
  Object.defineProperty(scrollContainer, "scrollHeight", {
    configurable: true,
    value: 2400,
  });
  scrollContainer.scrollTop = 0;
  return scrollContainer;
}

describe("TraceGridBentoCard card-level scrolling", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("keeps vertical scroll on the trace card body instead of the dispatch grid root", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const scrollContainer = findTraceCardScrollContainer(card);
    const traceGridRoot = scrollContainer.firstElementChild;
    const tableRegion = card.querySelector("[data-trace-dispatch-table]");

    expect(scrollContainer.className).toContain("overflow-auto");
    expect(traceGridRoot).toBeTruthy();
    expect(traceGridRoot?.className).not.toContain("overflow-x-clip");
    expectNoVerticalScrollContainer(traceGridRoot as Element);
    if (tableRegion) {
      expectNoVerticalScrollContainer(tableRegion);
    }
  });

  it("scrolls the trace card body from the dispatch table without scrolling the table wrapper", () => {
    render(
      <div style={{ height: 240, overflow: "hidden" }}>
        <TraceGridBentoCard
          state={{ status: "ready", trace: longDispatchTrace }}
        />
      </div>,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const scrollContainer = constrainTraceCardHeight(card, 240);
    const tableRegion = card.querySelector("[data-trace-dispatch-table]");
    if (!(tableRegion instanceof HTMLElement)) {
      throw new Error("Expected trace dispatch table region to render.");
    }

    scrollContainer.scrollTop = 280;

    expect(scrollContainer.scrollTop).toBe(280);
    expect(tableRegion.scrollTop).toBe(0);
    expect(scrollContainer.scrollHeight).toBeGreaterThan(
      scrollContainer.clientHeight,
    );
  });

  it("keeps fixed-height graph shells while the card body scrolls", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const graphFrames = card.querySelectorAll("[data-dashboard-graph-frame]");
    expect(graphFrames.length).toBeGreaterThan(0);
    for (const frame of graphFrames) {
      if (!(frame instanceof HTMLElement)) {
        continue;
      }
      expect(frame.style.height).not.toBe("");
      expect(frame.className).toContain("overflow-hidden");
      expectNoVerticalScrollContainer(frame);
    }

    const scrollContainer = constrainTraceCardHeight(card, 240);
    scrollContainer.scrollTop = 320;
    expect(within(card).getByText("Dispatch flow")).toBeTruthy();
    expect(scrollContainer.scrollTop).toBe(320);
  });
});
