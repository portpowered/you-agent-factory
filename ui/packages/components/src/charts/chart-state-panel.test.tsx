// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";

import { ChartStatePanel, type ChartStateStatus } from "./chart-state-panel";

const stateFixtures: Array<{
  description: string;
  role: "alert" | "status";
  status: ChartStateStatus;
  title: string;
}> = [
  {
    description: "Waiting for chart data to load.",
    role: "status",
    status: "loading",
    title: "Loading chart data",
  },
  {
    description: "No data points are available for this range.",
    role: "status",
    status: "empty",
    title: "No chart data",
  },
  {
    description: "The chart request failed. Try again later.",
    role: "alert",
    status: "error",
    title: "Unable to load chart",
  },
  {
    description: "The chart finished loading successfully.",
    role: "status",
    status: "success",
    title: "Chart ready",
  },
];

describe("ChartStatePanel", () => {
  it.each(stateFixtures)(
    "renders $status state with caller copy and $role semantics",
    ({ description, role, status, title }) => {
      renderPackageComponent(
        <ChartStatePanel
          description={description}
          status={status}
          title={title}
        />,
      );

      const panel = screen.getByRole(role);
      expect(panel).toHaveAttribute("data-chart-state", status);
      expect(panel).toHaveAttribute("data-chart-presentation", "standalone");
      expect(
        screen.getByRole("heading", { level: 3, name: title }),
      ).toBeInTheDocument();
      expect(screen.getByText(description)).toBeInTheDocument();
    },
  );

  it("marks loading state as busy and exposes a skeleton placeholder", () => {
    renderPackageComponent(
      <ChartStatePanel
        description="Waiting for chart data to load."
        status="loading"
        title="Loading chart data"
      />,
    );

    const loadingPanel = screen.getByRole("status");
    expect(loadingPanel).toHaveAttribute("aria-busy", "true");
    expect(loadingPanel.querySelector(".animate-pulse")).toBeTruthy();
  });

  it("uses assertive live region semantics for error states", () => {
    renderPackageComponent(
      <ChartStatePanel
        description="The chart request failed."
        status="error"
        title="Unable to load chart"
      />,
    );

    expect(screen.getByRole("alert")).toHaveAttribute("aria-live", "assertive");
  });

  it("renders optional caller-supplied actions", () => {
    renderPackageComponent(
      <ChartStatePanel
        action={<button type="button">Retry chart load</button>}
        description="The chart request failed."
        status="error"
        title="Unable to load chart"
      />,
    );

    expect(
      screen.getByRole("button", { name: "Retry chart load" }),
    ).toBeInTheDocument();
  });

  it("renders embedded presentation without the standalone dashed shell", () => {
    renderPackageComponent(
      <ChartStatePanel
        description="No data points are available for this range."
        presentation="embedded"
        status="empty"
        title="No chart data"
      />,
    );

    const panel = screen.getByRole("status");
    expect(panel).toHaveAttribute("data-chart-presentation", "embedded");
    expect(panel.className).not.toContain("border-dashed");
  });

  it("keeps state panels constrained inside narrow containers", () => {
    renderPackageComponent(
      <div className="w-64 overflow-hidden">
        <ChartStatePanel
          description="A long description that should wrap instead of forcing horizontal overflow in narrow chart containers."
          presentation="embedded"
          status="empty"
          title="No chart data"
        />
      </div>,
    );

    const panel = screen.getByRole("status");
    expect(panel.className).toContain("min-w-0");
    expect(panel.scrollWidth).toBeLessThanOrEqual(
      panel.parentElement?.clientWidth ?? 0,
    );
  });
});
