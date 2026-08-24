import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

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
const roleTokensSourcePath = path.join(
  packageStylesDir,
  "color-role-tokens.css",
);
const dashboardStylesSourcePath = path.join(stylesDir, "..", "styles.css");
const foundationPresetsSourcePath = path.join(
  packageStylesDir,
  "color-palette-presets.css",
);

const PRODUCT_AF_ROLE_PAIRS: ReadonlyArray<
  readonly [afToken: string, roleToken: string]
> = [
  ["--color-af-background", "--color-background"],
  ["--color-af-surface", "--color-surface"],
  ["--color-af-surface-subtle", "--color-surface-container-low"],
  ["--color-af-surface-raised", "--color-surface-container-high"],
  ["--color-af-border", "--color-outline"],
  ["--color-af-border-strong", "--color-outline-variant"],
  ["--color-af-text", "--color-on-surface"],
  ["--color-af-text-muted", "--color-on-surface-variant"],
  ["--color-af-text-subtle", "--color-on-surface-subtle"],
  ["--color-af-text-disabled", "--color-on-surface-disabled"],
  ["--color-af-text-inverse", "--color-on-inverse"],
  ["--color-af-code-ink", "--color-code"],
  ["--color-af-accent", "--color-primary"],
  ["--color-af-accent-hover", "--color-on-primary-container"],
  ["--color-af-accent-surface", "--color-primary-container"],
  ["--color-af-accent-border", "--color-primary"],
  ["--color-af-on-accent", "--color-on-primary"],
  ["--color-af-success", "--color-success"],
  ["--color-af-success-surface", "--color-success-container"],
  ["--color-af-success-text", "--color-on-success-container"],
  ["--color-af-on-success", "--color-on-success"],
  ["--color-af-warning", "--color-warning"],
  ["--color-af-warning-surface", "--color-warning-container"],
  ["--color-af-warning-text", "--color-on-warning-container"],
  ["--color-af-on-warning", "--color-on-warning"],
  ["--color-af-danger", "--color-error"],
  ["--color-af-danger-surface", "--color-error-container"],
  ["--color-af-danger-text", "--color-on-error-container"],
  ["--color-af-on-danger", "--color-on-error"],
  ["--color-af-info", "--color-info"],
  ["--color-af-info-surface", "--color-info-container"],
  ["--color-af-info-text", "--color-on-info-container"],
  ["--color-af-on-info", "--color-on-info"],
  ["--color-af-worker", "--color-tertiary"],
  ["--color-af-worker-surface", "--color-tertiary-container"],
  ["--color-af-worker-text", "--color-on-tertiary-container"],
];

const CHART_ROLE_PAIRS: ReadonlyArray<
  readonly [afToken: string, roleToken: string]
> = [
  ["--color-af-chart-grid-line", "--color-chart-grid-line"],
  ["--color-af-chart-queued", "--color-chart-queued"],
  ["--color-af-chart-in-flight", "--color-chart-in-flight"],
  ["--color-af-chart-completed", "--color-chart-completed"],
  ["--color-af-chart-failed", "--color-chart-failed"],
  ["--color-af-chart-failure-trend", "--color-chart-failure-trend"],
  ["--color-af-chart-rework-trend", "--color-chart-rework-trend"],
  ["--color-af-chart-timing-trend", "--color-chart-timing-trend"],
  ["--color-af-chart-selection-fill", "--color-chart-selection-fill"],
  ["--color-af-chart-selection-stroke", "--color-chart-selection-stroke"],
  ["--color-af-chart-cursor", "--color-chart-cursor"],
  ["--color-af-chart-active-dot-stroke", "--color-chart-active-dot-stroke"],
];

const GRAPH_OVERLAY_ROLE_PAIRS: ReadonlyArray<
  readonly [afToken: string, roleToken: string]
> = [
  ["--color-af-overlay", "--color-overlay"],
  ["--color-af-overlay-subtle", "--color-overlay-subtle"],
  ["--color-af-overlay-focus", "--color-overlay-focus"],
  ["--color-af-overlay-strong", "--color-overlay-strong"],
  ["--color-af-focus-ring", "--color-focus-ring"],
  ["--color-af-edge-muted", "--color-edge-muted"],
  ["--color-af-edge-muted-soft", "--color-edge-muted-soft"],
  ["--color-af-edge-danger-muted", "--color-edge-danger-muted"],
  ["--color-af-graph-controls-surface", "--color-graph-controls-surface"],
  [
    "--color-af-graph-controls-button-surface",
    "--color-graph-controls-button-surface",
  ],
  [
    "--color-af-graph-controls-button-surface-hover",
    "--color-graph-controls-button-surface-hover",
  ],
  ["--color-af-graph-controls-border", "--color-graph-controls-border"],
  ["--color-af-graph-controls-text", "--color-graph-controls-text"],
  [
    "--color-af-graph-controls-text-hover",
    "--color-graph-controls-text-hover",
  ],
  ["--color-af-graph-focus-indicator", "--color-graph-focus-indicator"],
];

function normalizeCssWhitespace(source: string): string {
  return source
    .replace(/\s+/g, " ")
    .replace(/\( /g, "(")
    .replace(/ \)/g, ")")
    .trim();
}

function parseHexColor(hex: string): readonly [number, number, number] {
  const normalized = hex.trim().replace("#", "");
  if (normalized.length !== 6) {
    throw new Error(`expected 6-digit hex, got ${hex}`);
  }
  const value = Number.parseInt(normalized, 16);
  return [(value >> 16) & 0xff, (value >> 8) & 0xff, value & 0xff] as const;
}

function relativeSaturation([r, g, b]: readonly [
  number,
  number,
  number,
]): number {
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  if (max === 0) {
    return 0;
  }
  return (max - min) / max;
}

function readFoundationHex(stylesSource: string, token: string): string {
  const match = stylesSource.match(
    new RegExp(
      `${token.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}:\\s*(#[0-9a-fA-F]{6})`,
    ),
  );
  if (!match?.[1]) {
    throw new Error(`missing ${token} hex in color-palette-presets.css`);
  }
  return match[1];
}

describe("color-role-tokens product af-* wiring", () => {
  it("maps each product af-* token to its Material role in color-role-tokens.css", () => {
    const source = readFileSync(roleTokensSourcePath, "utf8");

    for (const [afToken, roleToken] of PRODUCT_AF_ROLE_PAIRS) {
      expect(source).toContain(`${afToken}: var(${roleToken});`);
    }
  });

  it("maps each dashboard chart alias to a shared chart role", () => {
    const roleSource = readFileSync(roleTokensSourcePath, "utf8");
    const dashboardSource = readFileSync(dashboardStylesSourcePath, "utf8");

    for (const [afToken, roleToken] of CHART_ROLE_PAIRS) {
      expect(roleSource).toContain(`${afToken}: var(${roleToken});`);
      expect(dashboardSource).not.toContain(`${afToken}:`);
    }
  });

  it("maps graph, focus, and overlay aliases to shared roles", () => {
    const roleSource = normalizeCssWhitespace(
      readFileSync(roleTokensSourcePath, "utf8"),
    );
    const dashboardSource = readFileSync(dashboardStylesSourcePath, "utf8");

    for (const [afToken, roleToken] of GRAPH_OVERLAY_ROLE_PAIRS) {
      expect(roleSource).toContain(`${afToken}: var(${roleToken});`);
      expect(dashboardSource).not.toContain(`${afToken}:`);
    }
  });
});

describe("color-role-tokens accent rebalance (US-003)", () => {
  const roleTokensSource = readFileSync(roleTokensSourcePath, "utf8");
  const stylesSource = readFileSync(foundationPresetsSourcePath, "utf8");

  it("routes secondary and tertiary roles through calmer foundation accent keys", () => {
    expect(roleTokensSource).toContain(
      "--color-secondary: var(--color-af-foundation-secondary-accent);",
    );
    expect(roleTokensSource).toContain(
      "--color-tertiary: var(--color-af-foundation-tertiary-accent);",
    );
    expect(roleTokensSource).toContain(
      "--color-info: var(--color-af-foundation-info);",
    );
  });

  it("keeps secondary and tertiary calmer than legacy info/worker while primary stays strongest", () => {
    const primary = parseHexColor(
      readFoundationHex(stylesSource, "--color-af-foundation-accent"),
    );
    const secondary = parseHexColor(
      readFoundationHex(stylesSource, "--color-af-foundation-secondary-accent"),
    );
    const tertiary = parseHexColor(
      readFoundationHex(stylesSource, "--color-af-foundation-tertiary-accent"),
    );
    const info = parseHexColor(
      readFoundationHex(stylesSource, "--color-af-foundation-info"),
    );
    const worker = parseHexColor(
      readFoundationHex(stylesSource, "--color-af-foundation-worker"),
    );

    const primarySaturation = relativeSaturation(primary);
    const secondarySaturation = relativeSaturation(secondary);
    const tertiarySaturation = relativeSaturation(tertiary);
    const infoSaturation = relativeSaturation(info);
    const workerSaturation = relativeSaturation(worker);

    expect(secondarySaturation).toBeLessThan(infoSaturation);
    expect(tertiarySaturation).toBeLessThan(workerSaturation);
    expect(primarySaturation).toBeGreaterThan(secondarySaturation);
    expect(primarySaturation).toBeGreaterThan(tertiarySaturation);
  });
});
