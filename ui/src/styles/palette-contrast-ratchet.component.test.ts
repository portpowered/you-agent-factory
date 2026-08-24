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
  rgbEuclideanDistance,
  stableRatio,
} from "./palette-contrast-test-math";

const stylesDirectory = path.dirname(fileURLToPath(import.meta.url));
const stylesSourcePath = path.resolve(stylesDirectory, "..", "styles.css");
const CONTRAST_FLOOR = 4.5;
const BASELINE_PRECISION = 2;
const CHART_CONTRAST_FLOOR = 3;
const MIN_STATUS_SERIES_RGB_DISTANCE = 24;

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

const CHART_SERIES_CONTRAST_PAIRS = [
  ["queued", "--color-af-chart-queued"],
  ["in-flight", "--color-af-chart-in-flight"],
  ["completed", "--color-af-chart-completed"],
  ["failed", "--color-af-chart-failed"],
] as const;

const FACTORY_DARK_STATUS_COLORS = [
  [236, 191, 88],
  [181, 237, 244],
  [167, 240, 196],
  [255, 138, 138],
] as const;

interface PaletteContrastMeasurement {
  fillToken: string;
  foregroundToken: string;
  paletteId: ColorPaletteId;
  ratio: number;
  stableRatio: number;
}

interface ChartContrastMeasurement {
  fillToken: string;
  foregroundToken: string;
  paletteId: ColorPaletteId;
  property: "chart-series-on-canvas";
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

function measureChartSeriesContrast(
  paletteId: ColorPaletteId,
): ChartContrastMeasurement[] {
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
  const canvasToken = "--color-surface-container-low";
  const canvas = readColor(canvasToken);
  const canvasRgb = resolveFillRgb(canvasToken, canvas, surfaceRgb);

  return CHART_SERIES_CONTRAST_PAIRS.map(([, foregroundToken]) => {
    const foreground = readColor(foregroundToken);
    const foregroundRgb = compositeOver(foreground, canvasRgb);
    const ratio = contrastRatio(foregroundRgb, canvasRgb);
    return {
      fillToken: canvasToken,
      foregroundToken,
      paletteId,
      property: "chart-series-on-canvas",
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

function formatChartMeasurement(measurement: ChartContrastMeasurement): string {
  return [
    measurement.property,
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

beforeAll(async () => {
  const compiledCss = await compileDashboardStyles(stylesSourcePath);
  injectCompiledRootRules(compiledCss);
});

describe("exhaustive palette contrast ratchet", () => {
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
});

describe("dashboard chart palette contract", () => {
  it("measures every status series against the chart canvas in every palette", () => {
    const measurements = COLOR_PALETTE_IDS.flatMap(measureChartSeriesContrast);
    const failures = measurements.filter(
      (measurement) => measurement.ratio < CHART_CONTRAST_FLOOR,
    );

    console.log(
      `Measured ${measurements.length} chart-series-on-canvas cells (${COLOR_PALETTE_IDS.length} palettes x ${CHART_SERIES_CONTRAST_PAIRS.length} series).`,
    );
    console.log("property\tpalette\tforeground\tfill\tratio");
    for (const measurement of measurements) {
      console.log(formatChartMeasurement(measurement));
    }

    expect(
      failures,
      failures
        .map(
          (measurement) =>
            `${measurement.property} palette=${measurement.paletteId} foreground=${measurement.foregroundToken} fill=${measurement.fillToken} measured=${measurement.stableRatio.toFixed(BASELINE_PRECISION)} required floor=${CHART_CONTRAST_FLOOR.toFixed(1)}`,
        )
        .join("\n"),
    ).toEqual([]);
    expect(measurements).toHaveLength(20);
  });

  it("keeps queued, in-flight, completed, and failed colors distinguishable", () => {
    for (const paletteId of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteId);
      const computedStyle = getComputedStyle(document.documentElement);
      const colors = CHART_SERIES_CONTRAST_PAIRS.map(([series, token]) => ({
        color: resolveCssColor(paletteId, token, computedStyle),
        series,
        token,
      }));

      for (let firstIndex = 0; firstIndex < colors.length; firstIndex += 1) {
        for (
          let secondIndex = firstIndex + 1;
          secondIndex < colors.length;
          secondIndex += 1
        ) {
          const first = colors[firstIndex];
          const second = colors[secondIndex];
          if (!first || !second) {
            continue;
          }
          const distance = rgbEuclideanDistance(
            first.color.rgb,
            second.color.rgb,
          );
          console.log(
            `chart-series-color-distance\t${paletteId}\t${first.series}\t${second.series}\t${distance.toFixed(2)}\tminimum=${MIN_STATUS_SERIES_RGB_DISTANCE}`,
          );
          expect(
            distance,
            `${paletteId} ${first.token} and ${second.token} must be at least ${MIN_STATUS_SERIES_RGB_DISTANCE} sRGB units apart`,
          ).toBeGreaterThanOrEqual(MIN_STATUS_SERIES_RGB_DISTANCE);
        }
      }
    }
  });

  it("preserves the established Factory Dark status colors", () => {
    applyDocumentColorPalette("factory-dark");
    const computedStyle = getComputedStyle(document.documentElement);
    const colors = CHART_SERIES_CONTRAST_PAIRS.map(([, token]) =>
      resolveCssColor("factory-dark", token, computedStyle),
    );

    expect(colors.map(({ rgb }) => rgb)).toEqual(FACTORY_DARK_STATUS_COLORS);
  });
});

describe("palette token resolution diagnostics", () => {
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
