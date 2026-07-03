import { render, screen } from "@testing-library/react";
import {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
} from "@you-agent-factory/components/recipes";

import { WorkChartStatusPanel } from "./work-chart-status-panel";

describe("WorkChartStatusPanel", () => {
  it("renders standalone loading state with live status and skeleton affordance", () => {
    render(
      <WorkChartStatusPanel
        ariaBusy
        loading
        message="Loading chart samples."
        presentation="standalone"
        role="status"
        title="Loading"
      />,
    );

    const status = screen.getByRole("status");

    expect(status.getAttribute("aria-busy")).toBe("true");
    expect(status.getAttribute("aria-live")).toBe("polite");
    expect(status.getAttribute("data-work-chart-presentation")).toBe(
      "standalone",
    );
    expect(status.className).toContain("min-h-[14rem]");
    expect(status.querySelector(".animate-pulse")).toBeTruthy();
  });

  it("renders embedded error state without the dashed empty-state shell", () => {
    render(
      <WorkChartStatusPanel
        message="Chart data is unavailable."
        presentation="embedded"
        role="alert"
        title="Unable to render chart"
      />,
    );

    const alert = screen.getByRole("alert");

    expect(alert.getAttribute("aria-live")).toBe("assertive");
    expect(alert.getAttribute("data-work-chart-presentation")).toBe("embedded");
    expect(alert.className).toContain("min-h-[14rem]");
    expect(alert.className).not.toContain("border-dashed");
    expect(screen.getByText("Unable to render chart").className).toContain(
      WIDGET_FRAME_SECTION_HEADING_CLASS,
    );
    expect(screen.getByText("Chart data is unavailable.").className).toContain(
      WIDGET_FRAME_BODY_TEXT_CLASS,
    );
  });
});
