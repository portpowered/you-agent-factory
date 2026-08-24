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
const foundationPresetsSourcePath = path.join(
  packageStylesDir,
  "color-palette-presets.css",
);

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
