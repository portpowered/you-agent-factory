import { render, screen } from "@testing-library/react";

import { DashboardStatusPanel } from "./dashboard-status-panel";

describe("DashboardStatusPanel", () => {
  it("renders the default header state without optional detail copy", () => {
    const { container } = render(<DashboardStatusPanel title="Timeline unavailable" />);

    expect(screen.getByRole("heading", { name: "Timeline unavailable" })).toBeTruthy();
    expect(screen.getByText("Agent Factory")).toBeTruthy();
    expect(screen.queryByText("Waiting for more timeline data.")).toBeNull();
    expect(container.querySelector("section")?.className).not.toContain("border-af-danger/45");
  });

  it("renders the error tone and optional detail copy when provided", () => {
    const { container } = render(
      <DashboardStatusPanel
        detail="Waiting for more timeline data."
        title="Timeline unavailable"
        tone="error"
      />,
    );

    expect(screen.getByText("Waiting for more timeline data.")).toBeTruthy();
    expect(container.querySelector("section")?.className).toContain("border-af-danger/45");
  });
});
