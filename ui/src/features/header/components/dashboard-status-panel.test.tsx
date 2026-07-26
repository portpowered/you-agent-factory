import { render, screen } from "@testing-library/react";

import { Button } from "@you-agent-factory/components/primitives";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardStatusPanel } from "./dashboard-status-panel";

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
    expect(headerEyebrow?.className).toContain("text-primary");
    expect(heading.className).toContain("af-page-heading");
    expect(screen.queryByText("Waiting for more timeline data.")).toBeNull();
    expect(section?.getAttribute("data-dashboard-panel-shell")).toBe("panel");
    expect(section?.className).toContain("border-outline");
    expect(section?.className).toContain("bg-surface-container-high");
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
    expect(
      container
        .querySelector("section")
        ?.getAttribute("data-dashboard-panel-shell"),
    ).toBe("panel");
    expect(detail.className).toContain("max-w-80");
    expect(detail.className).toContain("text-on-error-container");
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
    expect(screen.getByText("U").className).not.toContain("sr-only");
  });

  it("renders optional recovery actions below the detail copy", () => {
    render(
      <DashboardStatusPanel
        actions={<Button>Retry clean replay</Button>}
        detail="Checkpoint reset required."
        title="Session recovery required"
        tone="error"
      />,
    );

    expect(
      screen.getByRole("button", { name: "Retry clean replay" }),
    ).toBeTruthy();
    expect(screen.getByText("Checkpoint reset required.")).toBeTruthy();
  });
});
