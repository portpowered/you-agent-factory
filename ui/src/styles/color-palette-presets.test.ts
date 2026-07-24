import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { getColorPaletteOptions } from "../features/header/messages/color-palette-options";
import { COLOR_PALETTE_IDS } from "../theme/color-palette";

const stylesDir = path.dirname(fileURLToPath(import.meta.url));
const packageStylesDir = path.resolve(
  stylesDir,
  "..",
  "..",
  "packages",
  "components",
  "src",
  "styles",
);
const palettePresetsSourcePath = path.join(
  packageStylesDir,
  "color-palette-presets.css",
);

const REQUIRED_FOUNDATION_KEYS = [
  "--color-af-foundation-background",
  "--color-af-foundation-accent",
  "--color-af-foundation-accent-ink",
  "--color-af-foundation-secondary-accent",
  "--color-af-foundation-tertiary-accent",
  "--color-af-foundation-success",
  "--color-af-foundation-warning",
  "--color-af-foundation-danger",
  "--color-af-foundation-info",
] as const;

describe("color-palette-presets (US-008)", () => {
  const palettePresetsSource = readFileSync(palettePresetsSourcePath, "utf8");

  it("documents exactly five predefined palettes", () => {
    expect(COLOR_PALETTE_IDS).toHaveLength(5);
    expect(getColorPaletteOptions("en").map((option) => option.label)).toEqual([
      "Factory Dark",
      "Factory Light",
      "Material Baseline",
      "Slate",
      "Olive",
    ]);
  });

  it.each(COLOR_PALETTE_IDS)(
    "defines foundation overrides for palette %s",
    (paletteId) => {
      expect(palettePresetsSource).toContain(
        `[data-color-palette="${paletteId}"]`,
      );

      for (const key of REQUIRED_FOUNDATION_KEYS) {
        const selectorPattern = new RegExp(
          `\\[data-color-palette="${paletteId}"\\][\\s\\S]*?${key.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}:\\s*#[0-9a-fA-F]{6}`,
        );
        expect(palettePresetsSource).toMatch(selectorPattern);
      }
    },
  );

  it("keeps yellow primary accent across all palette presets", () => {
    for (const paletteId of COLOR_PALETTE_IDS) {
      const match = palettePresetsSource.match(
        new RegExp(
          `\\[data-color-palette="${paletteId}"\\][\\s\\S]*?--color-af-foundation-accent:\\s*(#[0-9a-fA-F]{6})`,
        ),
      );
      expect(match?.[1]).toBe("#f5c76f");
    }
  });
});
