import { cleanup, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  buildLongDispatchTrace,
  constrainTraceCardHeight,
  expectNoVerticalScrollContainer,
  findTraceCardScrollContainer,
  findTraceDispatchTableRegion,
} from "../lib/trace-grid-card-scroll-test-helpers";
import { TraceGridBentoCard } from "./trace-grid-card";

const longDispatchTrace = buildLongDispatchTrace(12);

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
    const tableRegion = findTraceDispatchTableRegion(card);

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
