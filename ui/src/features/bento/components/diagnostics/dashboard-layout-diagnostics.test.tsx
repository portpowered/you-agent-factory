import { render, screen } from "@testing-library/react";

import { DashboardLayoutDiagnostics } from "./dashboard-layout-diagnostics";

describe("DashboardLayoutDiagnostics", () => {
  it("exposes an accessible success status when no issue exists", () => {
    render(<DashboardLayoutDiagnostics diagnostics={[]} />);

    expect(
      screen
        .getByRole("status")
        .getAttribute("data-dashboard-layout-diagnostics"),
    ).toBe("empty");
    expect(screen.getByText("Layout ready")).toBeTruthy();
    expect(screen.getByText("No layout issues detected.")).toBeTruthy();
  });

  it("exposes repair and storage failures in an accessible alert without raw identifiers", () => {
    render(
      <DashboardLayoutDiagnostics
        diagnostics={[
          { code: "invalid-id", count: 1, severity: "repair" },
          { code: "storage-unavailable", count: 1, severity: "error" },
        ]}
      />,
    );

    const alert = screen.getByRole("alert");
    expect(alert.getAttribute("data-dashboard-layout-diagnostics")).toBe(
      "issue",
    );
    expect(alert.textContent).toContain(
      "Unsafe card identities were replaced.",
    );
    expect(alert.textContent).toContain("Browser storage is unavailable");
    expect(alert.textContent).not.toContain("invalid-id");
    expect(alert.textContent).not.toContain("<unsafe>");
  });
});
