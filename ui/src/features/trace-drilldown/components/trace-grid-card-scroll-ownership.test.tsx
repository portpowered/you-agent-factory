import { cleanup, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  buildLongDispatchTrace,
  constrainTraceCardHeight,
  expectNoVerticalScrollBetweenDispatchTableAndCardBody,
  findTraceCardScrollContainer,
  findTraceDispatchTableRegion,
} from "../lib/trace-grid-card-scroll-test-helpers";
import { TraceGridBentoCard } from "./trace-grid-card";

const longDispatchTrace = buildLongDispatchTrace(12);

describe("TraceGridBentoCard dispatch scroll ownership regressions", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("keeps vertical scroll ownership on the card body for a long dispatch fixture", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expectNoVerticalScrollBetweenDispatchTableAndCardBody(card);
    expect(within(card).getAllByRole("row")).toHaveLength(13);
  });

  it("scrolls the card body when dispatch content exceeds the viewport height", () => {
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

  it("does not add nested vertical scrollports between the dispatch table and card body", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const scrollContainer = findTraceCardScrollContainer(card);
    const traceGridRoot = scrollContainer.firstElementChild;

    expect(traceGridRoot?.className).not.toContain("overflow-x-clip");
    expectNoVerticalScrollBetweenDispatchTableAndCardBody(card);
  });
});
