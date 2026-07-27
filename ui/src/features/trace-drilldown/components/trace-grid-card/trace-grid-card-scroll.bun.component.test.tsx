import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { cleanup, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import {
  buildLongDispatchTrace,
  expectNoVerticalScrollBetweenDispatchTableAndCardBody,
  expectNoVerticalScrollContainer,
  findTraceCardBody,
  findTraceDispatchTableRegion,
} from "../../lib/trace-grid-card-scroll-test-helpers";
import { TraceGridBentoCard } from "../trace-grid-card";

describe("TraceGridBentoCard scroll ownership", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("keeps a long dispatch table out of nested vertical scroll owners", () => {
    render(
      <div style={{ height: 240, overflow: "hidden" }}>
        <TraceGridBentoCard
          state={{ status: "ready", trace: buildLongDispatchTrace(12) }}
        />
      </div>,
    );

    const card = screen.getByRole("article", { name: "Trace drill-down" });
    const cardBody = findTraceCardBody(card);
    const tableRegion = findTraceDispatchTableRegion(card);

    expectNoVerticalScrollBetweenDispatchTableAndCardBody(card);
    expectNoVerticalScrollContainer(tableRegion, {
      requireOverflowYClip: true,
    });
    expect(tableRegion.hasAttribute("data-radix-scroll-area-viewport")).toBe(
      false,
    );
    expect(tableRegion.scrollTop).toBe(0);
    expect(cardBody.firstElementChild?.className).not.toContain(
      "overflow-x-clip",
    );
    expect(within(card).getAllByRole("row")).toHaveLength(13);
  });
});
