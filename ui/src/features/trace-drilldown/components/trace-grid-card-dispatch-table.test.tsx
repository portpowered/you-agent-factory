import { cleanup, render, screen, within } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  buildLongDispatchTrace,
  expectNoVerticalScrollContainer,
} from "../lib/trace-grid-card-scroll-test-helpers";
import { TraceGridBentoCard } from "./trace-grid-card";

const longDispatchTrace = buildLongDispatchTrace(10, "dispatch-long");

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

    expectNoVerticalScrollContainer(tableRegion, {
      requireOverflowYClip: true,
    });
    expect(within(card).getAllByRole("row")).toHaveLength(11);
    for (let index = 0; index < 10; index += 1) {
      expect(within(card).getByText(`dispatch-long-${index}`)).toBeTruthy();
    }
  });
});
