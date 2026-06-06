import { cleanup, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  buildLongDispatchTrace,
  expectNoVerticalScrollBetweenDispatchTableAndCardBody,
  expectPageFlowCardBody,
  findTraceCardBody,
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

  it("keeps long dispatch content in the page-flow card body", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    expectPageFlowCardBody(findTraceCardBody(card));
    expectNoVerticalScrollBetweenDispatchTableAndCardBody(card);
    expect(within(card).getAllByRole("row")).toHaveLength(13);
  });

  it("does not opt the trace body into a localized scroll viewport under a constrained ancestor", () => {
    render(
      <div style={{ height: 240, overflow: "hidden" }}>
        <TraceGridBentoCard
          state={{ status: "ready", trace: longDispatchTrace }}
        />
      </div>,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const cardBody = findTraceCardBody(card);
    const tableRegion = findTraceDispatchTableRegion(card);

    expectPageFlowCardBody(cardBody);
    expect(tableRegion.hasAttribute("data-radix-scroll-area-viewport")).toBe(
      false,
    );
    expect(tableRegion.scrollTop).toBe(0);
  });

  it("does not add nested vertical scrollports between the dispatch table and card body", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const cardBody = findTraceCardBody(card);
    const traceGridRoot = cardBody.firstElementChild;

    expect(traceGridRoot?.className).not.toContain("overflow-x-clip");
    expectNoVerticalScrollBetweenDispatchTableAndCardBody(card);
  });
});
