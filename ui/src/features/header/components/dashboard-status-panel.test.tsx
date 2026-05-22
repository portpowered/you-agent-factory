import { render, screen } from "@testing-library/react";

import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import { DashboardStatusPanel } from "./dashboard-status-panel";
import { getHeaderControlsMessages } from "../messages/header-controls";

describe("DashboardStatusPanel", () => {
  it("renders the default header state without optional detail copy", () => {
    const { container } = render(
      <DashboardStatusPanel title="Timeline unavailable" />,
    );
    const headerEyebrow = container.querySelector("p");

    expect(
      screen.getByRole("heading", { name: "Timeline unavailable" }),
    ).toBeTruthy();
    expect(screen.getByText("you-agent-factory").className).toContain("sr-only");
    expect(headerEyebrow?.textContent).toContain("∞");
    expect(headerEyebrow?.textContent).toContain("U");
    expect(screen.queryByText("Waiting for more timeline data.")).toBeNull();
    expect(container.querySelector("section")?.className).toContain(
      DASHBOARD_PANEL_SHELL_CLASS,
    );
  });

  it("renders the error tone through the shared shell and optional detail copy when provided", () => {
    const { container } = render(
      <DashboardStatusPanel
        detail="Waiting for more timeline data."
        title="Timeline unavailable"
        tone="error"
      />,
    );

    expect(screen.getByText("Waiting for more timeline data.")).toBeTruthy();
    expect(container.querySelector("section")?.className).toContain(
      DASHBOARD_PANEL_SHELL_CLASS,
    );
    expect(
      screen.getByText("Waiting for more timeline data.").className,
    ).toContain("text-af-danger-ink");
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
    expect(screen.getByText(messages.brandWordmark).className).toContain(
      "sr-only",
    );
  });
});
