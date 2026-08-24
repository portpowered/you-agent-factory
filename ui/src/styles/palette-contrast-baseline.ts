import type { ColorPaletteId } from "../theme/color-palette";

export interface PaletteContrastBaselineEntry {
  fillToken: string;
  foregroundToken: string;
  paletteId: ColorPaletteId;
  ratio: number;
}

/**
 * Measured current contrast debt, rounded to two decimal places so harmless
 * floating-point noise does not create a ratchet failure. An improvement that
 * changes the displayed ratio must lower or remove its entry.
 */
export const PALETTE_CONTRAST_BASELINE = [
  // WCAG 1.4.3 exempts disabled controls; these five measured entries are not
  // repair obligations and remain here to keep the complete table ratcheted.
  {
    paletteId: "factory-dark",
    foregroundToken: "--color-on-surface-disabled",
    fillToken: "--color-surface",
    ratio: 3.71,
  },
  {
    paletteId: "factory-light",
    foregroundToken: "--color-on-surface-disabled",
    fillToken: "--color-surface",
    ratio: 2.56,
  },
  {
    paletteId: "material-baseline",
    foregroundToken: "--color-on-surface-disabled",
    fillToken: "--color-surface",
    ratio: 3.4,
  },
  {
    paletteId: "slate",
    foregroundToken: "--color-on-surface-disabled",
    fillToken: "--color-surface",
    ratio: 3.62,
  },
  {
    paletteId: "olive",
    foregroundToken: "--color-on-surface-disabled",
    fillToken: "--color-surface",
    ratio: 3.7,
  },
  {
    paletteId: "factory-dark",
    foregroundToken: "--color-on-secondary",
    fillToken: "--color-secondary",
    ratio: 4.48,
  },
  {
    paletteId: "factory-dark",
    foregroundToken: "--color-on-tertiary",
    fillToken: "--color-tertiary",
    ratio: 3.58,
  },
  {
    paletteId: "material-baseline",
    foregroundToken: "--color-on-tertiary",
    fillToken: "--color-tertiary",
    ratio: 4.27,
  },
] as const satisfies ReadonlyArray<PaletteContrastBaselineEntry>;
