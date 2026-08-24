// @vitest-environment happy-dom

import path from "node:path";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, it } from "vitest";
import { compileDashboardStyles } from "../test-support/compile-dashboard-styles";
import { applyDocumentColorPalette } from "../theme/app-color-palette";
import { COLOR_PALETTE_IDS, type ColorPaletteId } from "../theme/color-palette";
import { PALETTE_CONTRAST_BASELINE } from "./palette-contrast-baseline";
import {
  type CssVariableReader,
  compositeOver,
  contrastRatio,
  type ParsedCssColor,
  resolveCssColor,
  resolveFillRgb,
  stableRatio,
} from "./palette-contrast-test-math";

const stylesDirectory = path.dirname(fileURLToPath(import.meta.url));
const stylesSourcePath = path.resolve(stylesDirectory, "..", "styles.css");
const CONTRAST_FLOOR = 4.5;
const BASELINE_PRECISION = 2;

const REQUIRED_CONTRAST_PAIRS = [
  ["--color-on-surface", "--color-surface"],
  ["--color-on-surface-variant", "--color-surface"],
  ["--color-on-surface-subtle", "--color-surface"],
  ["--color-on-surface-disabled", "--color-surface"],
  ["--color-on-surface", "--color-background"],
  ["--color-on-surface-subtle", "--color-background"],
  ["--color-code", "--color-surface"],
  ["--color-on-primary", "--color-primary"],
  ["--color-on-primary-container", "--color-primary-container"],
  ["--color-on-secondary", "--color-secondary"],
  ["--color-on-secondary-container", "--color-secondary-container"],
  ["--color-on-tertiary", "--color-tertiary"],
  ["--color-on-tertiary-container", "--color-tertiary-container"],
  ["--color-on-success", "--color-success"],
  ["--color-on-success-container", "--color-success-container"],
  ["--color-on-warning", "--color-warning"],
  ["--color-on-warning-container", "--color-warning-container"],
  ["--color-on-error", "--color-error"],
  ["--color-on-error-container", "--color-error-container"],
  ["--color-on-info", "--color-info"],
  ["--color-on-info-container", "--color-info-container"],
] as const;

interface PaletteContrastMeasurement {
  fillToken: string;
  foregroundToken: string;
  paletteId: ColorPaletteId;
  ratio: number;
  stableRatio: number;
}

function injectCompiledRootRules(compiledCss: string): void {
  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const paletteBlocks =
    compiledCss.match(/\[data-color-palette="[^"]+"\][^{]*\{[^}]*\}/g) ?? [];
  const style = document.createElement("style");
  style.textContent = [...rootBlocks, ...paletteBlocks].join("\n");
  document.head.appendChild(style);
}

function measurementKey(measurement: {
  fillToken: string;
  foregroundToken: string;
  paletteId: string;
}): string {
  return `${measurement.paletteId}|${measurement.foregroundToken}|${measurement.fillToken}`;
}

function measurePaletteContrast(
  paletteId: ColorPaletteId,
): PaletteContrastMeasurement[] {
  applyDocumentColorPalette(paletteId);
  const computedStyle = getComputedStyle(document.documentElement);
  const colors = new Map<string, ParsedCssColor>();
  const readColor = (tokenName: string): ParsedCssColor => {
    const cached = colors.get(tokenName);
    if (cached) {
      return cached;
    }
    const color = resolveCssColor(paletteId, tokenName, computedStyle);
    colors.set(tokenName, color);
    return color;
  };
  const surface = readColor("--color-surface");
  const surfaceRgb = compositeOver(surface, surface.rgb);

  return REQUIRED_CONTRAST_PAIRS.map(([foregroundToken, fillToken]) => {
    const foreground = readColor(foregroundToken);
    const fill = readColor(fillToken);
    const fillRgb = resolveFillRgb(fillToken, fill, surfaceRgb);
    const foregroundRgb = compositeOver(foreground, fillRgb);
    const ratio = contrastRatio(foregroundRgb, fillRgb);
    return {
      fillToken,
      foregroundToken,
      paletteId,
      ratio,
      stableRatio: stableRatio(ratio, BASELINE_PRECISION),
    };
  });
}

function formatMeasurement(measurement: PaletteContrastMeasurement): string {
  return [
    measurement.paletteId,
    measurement.foregroundToken,
    measurement.fillToken,
    measurement.stableRatio.toFixed(BASELINE_PRECISION),
  ].join("\t");
}

function baselineMap(): Map<
  string,
  (typeof PALETTE_CONTRAST_BASELINE)[number]
> {
  return new Map(
    PALETTE_CONTRAST_BASELINE.map((entry) => [measurementKey(entry), entry]),
  );
}

function createVariableReader(
  values: Readonly<Record<string, string>>,
): CssVariableReader {
  return {
    getPropertyValue: (name) => values[name] ?? "",
  };
}

describe("exhaustive palette contrast ratchet", () => {
  beforeAll(async () => {
    const compiledCss = await compileDashboardStyles(stylesSourcePath);
    injectCompiledRootRules(compiledCss);
  });

  it("measures all 105 role/palette cells and ratchets the current debt", () => {
    const measurements = COLOR_PALETTE_IDS.flatMap(measurePaletteContrast);
    const debt = measurements.filter(
      (measurement) => measurement.ratio < CONTRAST_FLOOR,
    );
    const baseline = baselineMap();
    const diagnostics: string[] = [];

    console.log(
      `Measured ${measurements.length} palette contrast cells (${COLOR_PALETTE_IDS.length} palettes x ${REQUIRED_CONTRAST_PAIRS.length} pairs).`,
    );
    console.log("palette\tforeground\tfill\tratio");
    for (const measurement of measurements) {
      console.log(formatMeasurement(measurement));
      const recorded = baseline.get(measurementKey(measurement));
      if (measurement.ratio < CONTRAST_FLOOR && !recorded) {
        diagnostics.push(
          `Unbaselined contrast debt: palette=${measurement.paletteId} foreground=${measurement.foregroundToken} fill=${measurement.fillToken} measured=${measurement.stableRatio.toFixed(BASELINE_PRECISION)} required floor=${CONTRAST_FLOOR.toFixed(1)}`,
        );
      }
      if (recorded && measurement.stableRatio > recorded.ratio) {
        diagnostics.push(
          `Contrast baseline improved: palette=${measurement.paletteId} foreground=${measurement.foregroundToken} fill=${measurement.fillToken} measured=${measurement.stableRatio.toFixed(BASELINE_PRECISION)} recorded=${recorded.ratio.toFixed(BASELINE_PRECISION)} required floor=${CONTRAST_FLOOR.toFixed(1)}; lower or remove the stale baseline entry.`,
        );
      }
    }

    const measuredKeys = new Set(measurements.map(measurementKey));
    for (const entry of PALETTE_CONTRAST_BASELINE) {
      if (!measuredKeys.has(measurementKey(entry))) {
        diagnostics.push(
          `Stale contrast baseline: palette=${entry.paletteId} foreground=${entry.foregroundToken} fill=${entry.fillToken} measured=missing required floor=${CONTRAST_FLOOR.toFixed(1)}; remove the stale baseline entry.`,
        );
      }
    }

    console.log(
      `Baselined sub-${CONTRAST_FLOOR} debt: ${debt.length} entries.`,
    );
    for (const paletteId of COLOR_PALETTE_IDS) {
      console.log(
        `${paletteId} sub-${CONTRAST_FLOOR} debt: ${debt.filter((measurement) => measurement.paletteId === paletteId).length} entries.`,
      );
    }
    expect(diagnostics, diagnostics.join("\n")).toEqual([]);
    expect(measurements).toHaveLength(105);
    expect(new Set(measurements.map(measurementKey)).size).toBe(105);
    expect(PALETTE_CONTRAST_BASELINE).toHaveLength(8);
    expect(debt).toHaveLength(8);
    expect(
      debt.filter(({ paletteId }) => paletteId === "factory-light"),
    ).toHaveLength(1);
    expect(
      measurements.find(
        ({ fillToken, foregroundToken, paletteId }) =>
          paletteId === "factory-light" &&
          foregroundToken === "--color-on-primary-container" &&
          fillToken === "--color-primary-container",
      )?.stableRatio,
    ).toBe(14.86);
  });

  it("fails unresolved, cyclic, and unsupported token values with context", () => {
    expect(() =>
      resolveCssColor(
        "factory-light",
        "--color-missing",
        createVariableReader({}),
      ),
    ).toThrow("palette=factory-light token=--color-missing");

    expect(() =>
      resolveCssColor(
        "factory-light",
        "--color-cycle-a",
        createVariableReader({
          "--color-cycle-a": "var(--color-cycle-b)",
          "--color-cycle-b": "var(--color-cycle-a)",
        }),
      ),
    ).toThrow("palette=factory-light token=--color-cycle-a");

    expect(() =>
      resolveCssColor(
        "factory-light",
        "--color-unsupported",
        createVariableReader({
          "--color-unsupported": "color(display-p3 1 0 0)",
        }),
      ),
    ).toThrow("palette=factory-light token=--color-unsupported");
  });
});
