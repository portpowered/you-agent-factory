import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { DashboardSessionTabsState } from "../hooks/use-dashboard-session-tabs-state";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardGeneralHeader } from "./dashboard-general-header";

vi.mock("./dashboard-header-color-palette-controls", () => ({
  DashboardHeaderColorPaletteControls: () => (
    <div data-testid="dashboard-header-color-palette-controls" />
  ),
}));

vi.mock("./dashboard-session-tabs", () => ({
  DashboardSessionTabs: () => <div data-testid="dashboard-session-tabs" />,
}));

describe("DashboardGeneralHeader", () => {
  it("keeps the semantic page heading and centers the brand lockup as a flex item", () => {
    const messages = getHeaderControlsMessages("en");

    render(
      <DashboardGeneralHeader
        locale="en"
        onChangeLocale={vi.fn()}
        sessionTabsState={{} as DashboardSessionTabsState}
      />,
    );

    const heading = screen.getByRole("heading", { level: 1 });
    const lockup = screen.getByAltText(messages.brandWordmark);

    expect(heading.className).toContain("flex");
    expect(heading.className).toContain("items-center");
    expect(heading.contains(lockup)).toBe(true);
    expect(lockup.getAttribute("alt")).toBe(messages.brandWordmark);
    expect(lockup.getAttribute("width")).toBe("48");
    expect(lockup.getAttribute("height")).toBe("48");
  });
});
