import { render, screen } from "@testing-library/react";

import { DashboardButton } from "./button";

describe("DashboardButton", () => {
  it("renders the primary tone with busy semantics for submit actions", () => {
    render(
      <DashboardButton busy type="submit">
        Submit work
      </DashboardButton>,
    );

    const button = screen.getByRole<HTMLButtonElement>("button", { name: "Submit work" });

    expect(button.getAttribute("aria-busy")).toBe("true");
    expect(button.getAttribute("type")).toBe("submit");
    expect(button.className).toContain("bg-af-accent");
  });

  it("renders the secondary tone for non-primary dashboard actions", () => {
    render(
      <DashboardButton disabled tone="secondary">
        Current
      </DashboardButton>,
    );

    const button = screen.getByRole<HTMLButtonElement>("button", { name: "Current" });

    expect(button.disabled).toBe(true);
    expect(button.className).toContain("bg-af-accent/10");
  });
});
