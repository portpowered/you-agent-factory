export const COLOR_PALETTE_IDS = [
  "factory-dark",
  "factory-light",
  "material-baseline",
  "slate",
  "olive",
] as const;

export type ColorPaletteId = (typeof COLOR_PALETTE_IDS)[number];

export const DEFAULT_COLOR_PALETTE: ColorPaletteId = "factory-dark";

export const COLOR_PALETTE_STORAGE_KEY = "infinite-you-dashboard-color-palette";

export interface ColorPaletteOption {
  id: ColorPaletteId;
  label: string;
}

export const COLOR_PALETTE_OPTIONS: readonly ColorPaletteOption[] = [
  { id: "factory-dark", label: "Factory Dark" },
  { id: "factory-light", label: "Factory Light" },
  { id: "material-baseline", label: "Material Baseline" },
  { id: "slate", label: "Slate" },
  { id: "olive", label: "Olive" },
] as const;

export function isColorPaletteId(value: string): value is ColorPaletteId {
  return (COLOR_PALETTE_IDS as readonly string[]).includes(value);
}

export function resolveColorPaletteId(palette?: string | null): ColorPaletteId {
  if (palette && isColorPaletteId(palette)) {
    return palette;
  }

  return DEFAULT_COLOR_PALETTE;
}
