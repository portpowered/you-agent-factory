import { render, screen } from "@testing-library/react";

import { DashboardPanelShell } from "./dashboard-shell";

describe("DashboardPanelShell", () => {
  it("renders the shared dashboard panel surface with a public shell marker", () => {
    render(
      <DashboardPanelShell aria-label="Factory status" shellKind="status">
        Status panel
      </DashboardPanelShell>,
    );

    const panel = screen.getByRole("region", { name: "Factory status" });
    expect(panel.getAttribute("data-dashboard-panel-shell")).toBe("status");
    expect(panel.className).toContain("border-outline");
    expect(panel.className).toContain("bg-surface-container-high");
    expect(panel.className).toContain("shadow-af-card");
  });

  it("supports article shells for widget cards", () => {
    render(
      <DashboardPanelShell as="article" aria-label="Work totals">
        Widget panel
      </DashboardPanelShell>,
    );

    expect(screen.getByRole("article", { name: "Work totals" })).toBeTruthy();
  });

  it("supports shared card inset spacing through the component contract", () => {
    render(
      <DashboardPanelShell aria-label="Inset card" inset>
        Inset panel
      </DashboardPanelShell>,
    );

    const panel = screen.getByRole("region", { name: "Inset card" });

    expect(panel.className).toContain("p-layout-inset-card");
    expect(panel.className).toContain("md:p-layout-inset-card-relaxed");
  });
});
