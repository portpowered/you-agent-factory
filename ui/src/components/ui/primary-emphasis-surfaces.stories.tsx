import { expect, within } from "storybook/test";

import { applyDocumentColorPalette } from "../../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../../theme/color-palette";
import { PrimaryEmphasisSurfacesShowcase } from "./primary-emphasis-surfaces";

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
  title: "Agent Factory/UI/Primary Emphasis Surfaces",
  component: PrimaryEmphasisSurfacesShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <PrimaryEmphasisSurfacesShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", { name: "Primary emphasis surfaces" }),
    ).toBeVisible();
    await expect(
      canvas.getByLabelText("Primary emphasis runtime surfaces"),
    ).toBeVisible();
  },
};

export const PaletteSwitching = {
  render: () => <PrimaryEmphasisSurfacesShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    for (const paletteId of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteId);

      await expect(document.documentElement.dataset.colorPalette).toBe(
        paletteId,
      );

      const statusPill = canvas.getByTestId("status-pill-active");
      const selectedButton = canvas.getByTestId("current-selection-selected");
      const selectedMenuItem = canvas.getByRole("menuitemradio", {
        name: "Factory Light",
      });

      expectAccentInkForeground(statusPill);
      expectPrimaryContainerBackground(statusPill);
      expectAccentInkForeground(selectedButton);
      expectPrimaryContainerBackground(selectedButton);
      expectAccentInkForeground(selectedMenuItem);
      expectPrimaryContainerBackground(selectedMenuItem);

      if (paletteId === "factory-light") {
        expect(normalizeColor(window.getComputedStyle(selectedMenuItem).color)).not.toBe(
          FACTORY_LIGHT_ACCENT_COLOR,
        );
      }
    }

    applyDocumentColorPalette("factory-dark");
  },
};
