import { readdirSync, readFileSync } from "node:fs";
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
const uiRoot = path.resolve(stylesDir, "..", "..");

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

const ALL_FOUNDATION_KEYS = [
  "--color-af-foundation-background",
  "--color-af-foundation-background-start",
  "--color-af-foundation-background-mid",
  "--color-af-foundation-canvas",
  "--color-af-foundation-surface",
  "--color-af-foundation-ink",
  "--color-af-foundation-code-ink",
  "--color-af-foundation-overlay",
  "--color-af-foundation-accent",
  "--color-af-foundation-accent-strong",
  "--color-af-foundation-accent-ink",
  "--color-af-foundation-info",
  "--color-af-foundation-info-strong",
  "--color-af-foundation-info-bright",
  "--color-af-foundation-info-ink",
  "--color-af-foundation-secondary-accent",
  "--color-af-foundation-secondary-accent-ink",
  "--color-af-foundation-tertiary-accent",
  "--color-af-foundation-tertiary-accent-ink",
  "--color-af-foundation-success",
  "--color-af-foundation-success-ink",
  "--color-af-foundation-warning",
  "--color-af-foundation-warning-ink",
  "--color-af-foundation-danger",
  "--color-af-foundation-danger-bright",
  "--color-af-foundation-danger-ink",
  "--color-af-foundation-worker",
  "--color-af-foundation-worker-ink",
] as const;

const CSS_SOURCE_DIRECTORY_EXCLUSIONS = new Set([
  ".git",
  ".vitest-reports",
  ".vitest-report-timings",
  ".warning-inventory",
  "coverage",
  "dist",
  "fallback_dist",
  "node_modules",
  "storybook-static",
]);

function cssSourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (CSS_SOURCE_DIRECTORY_EXCLUSIONS.has(entry.name)) {
        return [];
      }
      return cssSourceFiles(entryPath);
    }
    return entry.isFile() && entry.name.endsWith(".css") ? [entryPath] : [];
  });
}

function literalFoundationOwners(): Map<string, Set<string>> {
  const owners = new Map<string, Set<string>>();
  const declarationPattern =
    /(--color-af-foundation-[a-z0-9-]+)\s*:\s*([^;{}]+)(?:;|$)/gi;

  for (const sourceFile of cssSourceFiles(uiRoot)) {
    const source = readFileSync(sourceFile, "utf8").replace(
      /\/\*[\s\S]*?\*\//g,
      "",
    );
    for (const match of source.matchAll(declarationPattern)) {
      const value = match[2]?.trim();
      if (!value || value.startsWith("var(")) {
        continue;
      }
      const token = match[1];
      const relativeSourceFile = path.relative(uiRoot, sourceFile);
      const tokenOwners = owners.get(token) ?? new Set<string>();
      tokenOwners.add(relativeSourceFile);
      owners.set(token, tokenOwners);
    }
  }

  return owners;
}

function nonCanonicalFoundationOwnerDiagnostics(
  owners: Map<string, Set<string>>,
  canonicalSourceFile: string,
): string[] {
  return [...owners.entries()]
    .filter(
      ([, tokenOwners]) =>
        tokenOwners.size !== 1 || !tokenOwners.has(canonicalSourceFile),
    )
    .map(
      ([token, tokenOwners]) =>
        `${token}: ${[...tokenOwners].sort().join(", ")}`,
    );
}

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

  it("keeps literal foundation values owned by one CSS source file", () => {
    const owners = literalFoundationOwners();
    const canonicalSourceFile = path.relative(uiRoot, palettePresetsSourcePath);
    const missingTokens = ALL_FOUNDATION_KEYS.filter(
      (token) => !owners.has(token),
    );
    const wrongOwners = ALL_FOUNDATION_KEYS.filter((token) => {
      const tokenOwners = owners.get(token);
      return tokenOwners?.size !== 1 || !tokenOwners.has(canonicalSourceFile);
    });
    const duplicates = [...owners.entries()]
      .filter(([, tokenOwners]) => tokenOwners.size > 1)
      .map(
        ([token, tokenOwners]) =>
          `${token}: ${[...tokenOwners].sort().join(", ")}`,
      );
    const nonCanonicalOwners = nonCanonicalFoundationOwnerDiagnostics(
      owners,
      canonicalSourceFile,
    );

    expect(
      missingTokens,
      `Missing literal foundation tokens: ${missingTokens.join(", ")}`,
    ).toEqual([]);
    expect(
      duplicates,
      `A literal foundation token must not have multiple CSS owners:\n${duplicates.join(
        "\n",
      )}`,
    ).toEqual([]);
    expect(
      wrongOwners,
      `Expected literal foundation tokens in ${canonicalSourceFile}, got:\n${wrongOwners
        .map(
          (token) => `${token}: ${[...(owners.get(token) ?? [])].join(", ")}`,
        )
        .join("\n")}`,
    ).toEqual([]);
    expect(
      nonCanonicalOwners,
      `Every literal foundation token must be owned only by ${canonicalSourceFile}; got:\n${nonCanonicalOwners.join(
        "\n",
      )}`,
    ).toEqual([]);
  });

  it("reports a newly introduced literal foundation token in a non-canonical CSS file", () => {
    const token = "--color-af-foundation-new";
    const nonCanonicalSourceFile = path.join("src", "styles.css");
    const canonicalSourceFile = path.relative(uiRoot, palettePresetsSourcePath);
    const owners = new Map([[token, new Set([nonCanonicalSourceFile])]]);

    expect(
      nonCanonicalFoundationOwnerDiagnostics(owners, canonicalSourceFile),
    ).toEqual([`${token}: ${nonCanonicalSourceFile}`]);
  });
});
