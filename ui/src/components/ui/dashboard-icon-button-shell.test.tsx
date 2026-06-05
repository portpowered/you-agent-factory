import { render, screen } from "@testing-library/react";

import { DashboardIconButtonShell } from "./dashboard-icon-button-shell";

describe("DashboardIconButtonShell", () => {
  it("provides a reusable danger ghost tone for destructive icon actions", () => {
    render(
      <DashboardIconButtonShell aria-label="Remove item" tone="dangerGhost">
        <span aria-hidden="true">x</span>
      </DashboardIconButtonShell>,
    );

    const button = screen.getByRole("button", { name: "Remove item" });

    expect(button.className).toContain("hover:border-af-danger-border");
    expect(button.className).toContain("hover:bg-error-container");
    expect(button.className).toContain("hover:text-on-error-container");
    expect(button.className).toContain("rounded-lg");
  });
});
