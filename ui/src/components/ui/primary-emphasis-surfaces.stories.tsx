import "../../styles.css";

import { expect, within } from "storybook/test";

import { CurrentSelectionSelectableButton } from "../../features/current-selection/base/components/presentation/current-selection-selectable-button";
import {
  DashboardHeaderOptionMenuItem,
  DashboardHeaderOptionMenuSurface,
} from "../../features/header/components/dashboard-header-option-menu";
import { applyDocumentColorPalette } from "../../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../../theme/color-palette";
import { DashboardStatusPill } from "./dashboard-status-pill";

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

function PrimaryEmphasisSurfacesShowcase() {
  return (
    <div className="grid max-w-xl gap-6 rounded-2xl border border-outline bg-background p-6 text-on-surface">
      <header className="grid gap-2">
        <h2 className="m-0 font-display text-2xl tracking-[-0.03em]">
          Primary emphasis surfaces
        </h2>
        <p className="m-0 text-sm text-on-surface-variant">
          Shared dashboard primitives that should keep accent-ink foreground on
          primary-container emphasis after palette switches.
        </p>
      </header>

      <section
        aria-label="Primary emphasis runtime surfaces"
        className="grid gap-4"
      >
        <DashboardStatusPill data-testid="status-pill-active" tone="active">
          Active status
        </DashboardStatusPill>

        <CurrentSelectionSelectableButton
          data-testid="current-selection-selected"
          selected
        >
          Selected work item
        </CurrentSelectionSelectableButton>

        <DashboardHeaderOptionMenuSurface aria-label="Palette menu" role="menu">
          <DashboardHeaderOptionMenuItem isSelected onClick={() => undefined}>
            Factory Light
          </DashboardHeaderOptionMenuItem>
        </DashboardHeaderOptionMenuSurface>
      </section>
    </div>
  );
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

      const selectedMenuItem = canvas.getByRole("menuitemradio", {
        name: "Factory Light",
      });

      expectAccentInkForeground(selectedMenuItem);
      expectPrimaryContainerBackground(selectedMenuItem);

      if (paletteId === "factory-light") {
        expect(
          normalizeColor(window.getComputedStyle(selectedMenuItem).color),
        ).not.toBe(FACTORY_LIGHT_ACCENT_COLOR);
      }
    }

    applyDocumentColorPalette("factory-dark");
  },
};
