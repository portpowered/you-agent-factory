import { cleanup, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  buildLongDispatchTrace,
  expectNoVerticalScrollContainer,
  expectPageFlowCardBody,
  findTraceCardBody,
  findTraceDispatchTableRegion,
} from "../lib/trace-grid-card-scroll-test-helpers";
import { TraceGridBentoCard } from "./trace-grid-card";

const longDispatchTrace = buildLongDispatchTrace(12);

describe("TraceGridBentoCard page-flow scrolling", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders the trace card body in page flow instead of a ScrollArea viewport", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const cardBody = findTraceCardBody(card);
    const traceGridRoot = cardBody.firstElementChild;
    const tableRegion = card.querySelector("[data-trace-dispatch-table]");

    expectPageFlowCardBody(cardBody);
    expect(traceGridRoot).toBeTruthy();
    expect(traceGridRoot?.className).not.toContain("overflow-x-clip");
    expectNoVerticalScrollContainer(traceGridRoot as Element);
    if (tableRegion) {
      expectNoVerticalScrollContainer(tableRegion);
    }
  });

  it("renders long dispatch tables without a nested vertical scroll owner", () => {
    render(
      <TraceGridBentoCard
        state={{ status: "ready", trace: longDispatchTrace }}
      />,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const cardBody = findTraceCardBody(card);
    const tableRegion = findTraceDispatchTableRegion(card);

    expectPageFlowCardBody(cardBody);
    expectNoVerticalScrollContainer(tableRegion);
    expect(within(card).getAllByRole("row")).toHaveLength(13);
  });

  it("keeps fixed-height graph shells while the card body stays in page flow", () => {
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
      expect(
        frame.style.height !== "" || frame.parentElement?.getAttribute("style"),
      ).toBeTruthy();
      expect(frame.className).toContain("overflow-hidden");
      expectNoVerticalScrollContainer(frame);
    }

    const cardBody = findTraceCardBody(card);
    expect(within(card).getByText("Dispatch flow")).toBeTruthy();
    expectPageFlowCardBody(cardBody);
  });
});
