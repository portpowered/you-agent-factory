import "../../../styles.css";

import { expect, userEvent, within } from "storybook/test";

import { COLOR_PALETTE_IDS } from "../../../theme";
import { getColorPaletteOptions } from "../messages/color-palette-options";
import { getHeaderControlsMessages } from "../messages/header-controls";
import { DashboardHeader } from "./dashboard-header";

const ACCENT_INK_COLOR = "rgb(26,34,40)";
const FACTORY_LIGHT_ACCENT_COLOR = "rgb(245,199,111)";
const SECONDARY_TONE_SURFACE = "rgba(0,0,0,0.04)";

function normalizeColor(color: string): string {
  return color.replace(/\s+/g, "").toLowerCase();
}

function expectAccentInkForeground(element: HTMLElement): void {
  expect(normalizeColor(window.getComputedStyle(element).color)).toBe(
    ACCENT_INK_COLOR,
  );
}

function expectPrimaryContainerBackground(element: HTMLElement): void {
  const backgroundColor = normalizeColor(
    window.getComputedStyle(element).backgroundColor,
  );
  expect(backgroundColor).not.toBe("rgba(0,0,0,0)");
  expect(backgroundColor).not.toBe("transparent");
  expect(backgroundColor).not.toBe(SECONDARY_TONE_SURFACE);
}

export default {
  title: "you-agent-factory/Dashboard/Color Palette Selector",
  component: DashboardHeader,
  tags: ["test"],
};

export const PaletteOptions = {
  render: () => (
    <div style={{ margin: "0 auto", maxWidth: "1280px", width: "100%" }}>
      <p className="mb-3 text-on-surface-variant">
        Open the palette dropdown beside the language control to preview{" "}
        {COLOR_PALETTE_IDS.length} predefined palettes.
      </p>
      <DashboardHeader />
    </div>
  ),
};

export const PaletteMenuSelectedState = {
  render: () => (
    <div style={{ margin: "0 auto", maxWidth: "1280px", width: "100%" }}>
      <DashboardHeader />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const headerMessages = getHeaderControlsMessages("en");
    const paletteOptions = getColorPaletteOptions("en");

    const paletteButton = canvas.getByRole("button", {
      name: headerMessages.paletteMenuButtonLabel,
    });

    for (const paletteId of COLOR_PALETTE_IDS) {
      const selectedOption = paletteOptions.find(
        (option) => option.id === paletteId,
      );
      if (!selectedOption) {
        throw new Error(`missing palette option for ${paletteId}`);
      }

      await userEvent.click(paletteButton);
      await userEvent.click(
        canvas.getByRole("menuitemradio", { name: selectedOption.label }),
      );

      await userEvent.click(paletteButton);

      const selectedMenuItem = canvas.getByRole("menuitemradio", {
        name: selectedOption.label,
      });

      expect(selectedMenuItem.getAttribute("aria-checked")).toBe("true");
      expectAccentInkForeground(selectedMenuItem);
      expectPrimaryContainerBackground(selectedMenuItem);

      if (paletteId === "factory-light") {
        expect(
          normalizeColor(window.getComputedStyle(selectedMenuItem).color),
        ).not.toBe(FACTORY_LIGHT_ACCENT_COLOR);
      }

      await userEvent.keyboard("{Escape}");
    }
  },
};
