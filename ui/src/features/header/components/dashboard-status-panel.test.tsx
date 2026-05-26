import { render, screen } from "@testing-library/react";

import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import { DASHBOARD_PAGE_HEADING_CLASS } from "../../../components/ui/dashboard-typography";
import { DashboardStatusPanel } from "./dashboard-status-panel";
import { getHeaderControlsMessages } from "../messages/header-controls";

describe("DashboardStatusPanel", () => {
  it("renders the default header state without optional detail copy", () => {
    const { container } = render(
      <DashboardStatusPanel title="Timeline unavailable" />,
    );
    const headerEyebrow = container.querySelector("p");
    const section = container.querySelector("section");
    const heading = screen.getByRole("heading", {
      name: "Timeline unavailable",
    });

    expect(heading).toBeTruthy();
    expect(screen.getByText("U").className).not.toContain("sr-only");
    expect(headerEyebrow?.textContent).toContain("U");
    expect(headerEyebrow?.className).toContain("text-af-accent");
    expect(heading.className).toContain(DASHBOARD_PAGE_HEADING_CLASS);
    expect(screen.queryByText("Waiting for more timeline data.")).toBeNull();
    expect(section?.className).toContain(DASHBOARD_PANEL_SHELL_CLASS);
    expect(section?.className).toContain("mb-4");
  });

  it("renders the error tone through the shared shell and optional detail copy when provided", () => {
    const { container } = render(
      <DashboardStatusPanel
        detail="Waiting for more timeline data."
        title="Timeline unavailable"
        tone="error"
      />,
    );

    const detail = screen.getByText("Waiting for more timeline data.");

    expect(detail).toBeTruthy();
    expect(container.querySelector("section")?.className).toContain(
      DASHBOARD_PANEL_SHELL_CLASS,
    );
    expect(detail.className).toContain("max-w-80");
    expect(detail.className).toContain("text-af-danger-text");
  });

  it("resolves brand copy from the requested locale catalog", () => {
    const messages = getHeaderControlsMessages("zh-CN");

    render(
      <DashboardStatusPanel
        locale="zh-CN"
        title={messages.loadingDashboardTitle}
      />,
    );

    expect(
      screen.getByRole("heading", { name: messages.loadingDashboardTitle }),
    ).toBeTruthy();
    expect(screen.getByText("U").className).not.toContain(
      "sr-only",
    );
  });
});
