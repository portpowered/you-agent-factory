import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  DashboardHeaderOptionMenuItem,
  DashboardHeaderOptionMenuSurface,
} from "./dashboard-header-option-menu";

describe("DashboardHeaderOptionMenuSurface", () => {
  it("renders the shared floating header option menu surface", () => {
    render(
      <DashboardHeaderOptionMenuSurface
        aria-label="Theme"
        minWidthClassName="min-w-52"
        role="menu"
      >
        Menu body
      </DashboardHeaderOptionMenuSurface>,
    );

    const menu = screen.getByRole("menu", { name: "Theme" });

    expect(menu.className).toContain("rounded-2xl");
    expect(menu.className).toContain("bg-surface-container-high");
    expect(menu.className).toContain("shadow-af-panel");
    expect(menu.className).toContain("min-w-52");
  });
});

describe("DashboardHeaderOptionMenuItem", () => {
  it("renders selectable menu items with shared selected and unselected styling", () => {
    render(
      <>
        <DashboardHeaderOptionMenuItem isSelected onClick={() => undefined}>
          Selected
        </DashboardHeaderOptionMenuItem>
        <DashboardHeaderOptionMenuItem
          isSelected={false}
          onClick={() => undefined}
        >
          Unselected
        </DashboardHeaderOptionMenuItem>
      </>,
    );

    const selected = screen.getByRole("menuitemradio", { name: "Selected" });
    const unselected = screen.getByRole("menuitemradio", {
      name: "Unselected",
    });

    expect(selected.getAttribute("aria-checked")).toBe("true");
    expect(selected.className).toContain("bg-primary-container");
    expect(selected.className).toContain("text-on-primary");
    expect(unselected.getAttribute("aria-checked")).toBe("false");
    expect(unselected.className).toContain("text-on-surface-variant");
  });

  it("keeps selected menu items on ghost tone so primary-container classes win at runtime", () => {
    render(
      <DashboardHeaderOptionMenuItem isSelected onClick={() => undefined}>
        Selected
      </DashboardHeaderOptionMenuItem>,
    );

    const selected = screen.getByRole("menuitemradio", { name: "Selected" });

    expect(selected.className).toContain("bg-primary-container");
    expect(selected.className).toContain("text-on-primary");
    expect(selected.className).toContain("border-transparent bg-transparent");
    expect(selected.className).not.toContain("text-primary hover:border-primary");
  });
});
