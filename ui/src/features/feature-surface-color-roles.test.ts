import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const FEATURES_ROOT = join(import.meta.dirname);

function walkFeatures(dir: string): string[] {
  const files: string[] = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      files.push(...walkFeatures(path));
    } else if (/\.(tsx|ts)$/.test(name)) {
      if (
        /\.(test|stories)\.(tsx|ts)$/.test(name) ||
        name.includes(".coverage.test.")
      ) {
        continue;
      }
      files.push(path);
    }
  }
  return files;
}

/** Transitional tokens that US-009 migrated to Material role utilities. */
const FORBIDDEN_TRANSITIONAL_PATTERNS = [
  /\bbg-af-surface-(subtle|raised)\b/,
  /\bbg-af-surface\b/,
  /\bbg-af-background\b/,
  /\bbg-af-bg\b/,
  /\bborder-af-border\b/,
  /\bborder-af-border-strong\b/,
  /\btext-af-text(?!-)/,
  /\btext-af-text-muted\b/,
  /\bbg-af-accent-surface\b/,
  /\bborder-af-accent-border\b/,
  /\btext-af-accent\b/,
  /\bbg-af-accent\b/,
  /\btext-af-ink\b/,
  /\btext-af-success\b/,
  /\btext-af-danger\b/,
];

function relativeFeaturePath(absolutePath: string): string {
  return absolutePath.slice(FEATURES_ROOT.length + 1);
}

describe("feature surface color roles (US-009)", () => {
  const featureSources = walkFeatures(FEATURES_ROOT).filter(
    (path) => !path.endsWith("feature-surface-color-roles.test.ts"),
  );

  it.each(featureSources.map((path) => [relativeFeaturePath(path), path]))(
    "%s avoids transitional neutral and accent class tokens",
    (_label, filePath) => {
      const source = readFileSync(filePath, "utf8");

      for (const pattern of FORBIDDEN_TRANSITIONAL_PATTERNS) {
        expect(source).not.toMatch(pattern);
      }
    },
  );

  it("representative graph and header surfaces use role utilities", () => {
    const activityNodeShell = readFileSync(
      join(FEATURES_ROOT, "graphs/components/graph-node-shell.tsx"),
      "utf8",
    );
    const packageNodeShell = readFileSync(
      join(
        FEATURES_ROOT,
        "../../packages/components/src/graphs/graph-node-shell.tsx",
      ),
      "utf8",
    );
    const workstationNodeView = readFileSync(
      join(
        FEATURES_ROOT,
        "../../packages/factory-graph/src/semantic-workstation-node.tsx",
      ),
      "utf8",
    );
    const sessionTab = readFileSync(
      join(FEATURES_ROOT, "header/components/dashboard-session-tab.tsx"),
      "utf8",
    );
    const sessionTabs = readFileSync(
      join(FEATURES_ROOT, "header/components/dashboard-session-tabs.tsx"),
      "utf8",
    );

    expect(activityNodeShell).toContain("GraphNodeShell");
    expect(packageNodeShell).toContain("border-outline bg-surface");
    expect(packageNodeShell).toContain("text-on-surface");
    expect(workstationNodeView).toMatch(/\btext-on-surface(-subtle)?\b/);
    expect(workstationNodeView).toContain("border-outline bg-surface");
    expect(sessionTabs).toMatch(/\btext-on-surface(-variant)?\b/);
    expect(sessionTab).toMatch(/\bbg-surface-container-(low|high)\b/);
  });
});
