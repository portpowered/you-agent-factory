import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const stylesDir = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(stylesDir, "../..");
const repoRoot = path.resolve(uiRoot, "..");

const REGRESSION_CONTRACT_TESTS: ReadonlyArray<readonly [label: string, relativePath: string]> =
  [
    ["color role tokens", "src/styles/color-role-tokens.test.ts"],
    ["color role aliases", "src/styles/color-role-aliases.test.ts"],
    ["color palette presets", "src/styles/color-palette-presets.test.ts"],
    ["typography role tokens", "src/styles/typography-role-tokens.test.ts"],
    ["layout role tokens", "src/styles/layout-role-tokens.test.ts"],
    [
      "shared primitive semantics",
      "src/components/ui/shared-primitive-semantic-color-roles.test.ts",
    ],
    [
      "shared primitive neutrals",
      "src/components/ui/shared-primitive-neutral-surface-roles.test.ts",
    ],
    ["feature surfaces", "src/features/feature-surface-color-roles.test.ts"],
    ["dashboard graph chrome", "src/components/dashboard/dashboard-graph.test.tsx"],
  ];

const STORYBOOK_FIXTURES: ReadonlyArray<readonly [label: string, relativePath: string]> =
  [
    [
      "theme overview",
      "src/components/ui/theme-role-migration-overview.stories.tsx",
    ],
    [
      "accent contrast",
      "src/components/ui/color-role-accent-contrast.stories.tsx",
    ],
    [
      "neutral surfaces",
      "src/components/ui/color-role-neutral-surfaces.stories.tsx",
    ],
    [
      "typography hierarchy",
      "src/components/ui/typography-role-hierarchy.stories.tsx",
    ],
    ["layout primitives", "src/components/ui/layout-role-showcase.stories.tsx"],
    [
      "palette selector",
      "src/features/header/components/dashboard-palette-selector.stories.tsx",
    ],
    [
      "trace graph surfaces",
      "src/features/trace-drilldown/components/trace-graph-surfaces.stories.tsx",
    ],
    [
      "factory graph editor",
      "src/features/factory-graph-editor/components/factory-graph-editor-flow.stories.tsx",
    ],
  ];

describe("theme role migration regression index (US-010)", () => {
  it.each(REGRESSION_CONTRACT_TESTS)(
    "keeps regression contract %s at %s",
    (_label, relativePath) => {
      const absolutePath = path.join(uiRoot, relativePath);
      expect(existsSync(absolutePath)).toBe(true);
    },
  );

  it.each(STORYBOOK_FIXTURES)(
    "keeps Storybook fixture %s at %s",
    (_label, relativePath) => {
      const absolutePath = path.join(uiRoot, relativePath);
      expect(existsSync(absolutePath)).toBe(true);
    },
  );

  it("documents phased rollout and cleanup in the rollout guide", () => {
    const rolloutPath = path.join(
      repoRoot,
      "docs/internal/development/material-color-role-migration-rollout.md",
    );
    const source = readFileSync(rolloutPath, "utf8");

    expect(source).toContain("## Rollout order");
    expect(source).toContain("## Cleanup phase");
    expect(source).toMatch(/Taxonomy.*US-001/s);
    expect(source).toContain("color-role-aliases.css");
  });

  it("dashboard graph chrome uses Material role CSS variables", () => {
    const graphSource = readFileSync(
      path.join(uiRoot, "src/components/dashboard/dashboard-graph.tsx"),
      "utf8",
    );

    expect(graphSource).toContain("var(--color-outline)");
    expect(graphSource).toContain("var(--color-surface-container-high)");
    expect(graphSource).toContain("var(--color-on-surface)");
    expect(graphSource).toContain("var(--color-surface)");
  });
});
