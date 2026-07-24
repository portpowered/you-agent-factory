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
const stylesSourcePath = path.join(stylesDir, "..", "styles.css");

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
    throw new Error(`missing ${token} hex in styles.css`);
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
});

describe("color-role-tokens accent rebalance (US-003)", () => {
  const roleTokensSource = readFileSync(roleTokensSourcePath, "utf8");
  const stylesSource = readFileSync(stylesSourcePath, "utf8");

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
