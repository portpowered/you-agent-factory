import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const PROMPT_EDITOR_DIR = join(dirname(fileURLToPath(import.meta.url)));

/** Transitional neutral surface/border tokens migrated to Material role utilities. */
const FORBIDDEN_TRANSITIONAL_NEUTRAL_SURFACE_PATTERNS = [
  /\bbg-af-surface-(subtle|raised)\b/,
  /\bbg-af-surface\b/,
  /\bborder-af-border\b/,
  /\bborder-af-border-strong\b/,
  /\bbg-af-border\b/,
  /\bbg-af-border-strong\b/,
  /\bhover:bg-af-border-strong\b/,
];

/** Transitional neutral text tokens migrated to Material role utilities. */
const FORBIDDEN_TRANSITIONAL_NEUTRAL_TEXT_PATTERNS = [
  /\btext-af-text(?!-)/,
  /\btext-af-text-muted\b/,
];

function walkPromptEditorProductionSources(dir: string): string[] {
  const files: string[] = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      files.push(...walkPromptEditorProductionSources(path));
    } else if (/\.(tsx|ts)$/.test(name)) {
      if (
        /\.(test|stories)\.(tsx|ts)$/.test(name) ||
        name.includes(".coverage.test.")
      ) {
        continue;
      }
      if (name === "prompt-editor-neutral-surface-roles.test.ts") {
        continue;
      }
      files.push(path);
    }
  }
  return files;
}

function readPromptEditorSource(fileName: string): string {
  return readFileSync(join(PROMPT_EDITOR_DIR, fileName), "utf8");
}

function relativePromptEditorPath(absolutePath: string): string {
  return absolutePath.slice(PROMPT_EDITOR_DIR.length + 1);
}

function expectNoForbiddenTransitionalNeutralSurfaces(source: string): void {
  for (const pattern of FORBIDDEN_TRANSITIONAL_NEUTRAL_SURFACE_PATTERNS) {
    expect(source).not.toMatch(pattern);
  }
}

function expectNoForbiddenTransitionalNeutralText(source: string): void {
  for (const pattern of FORBIDDEN_TRANSITIONAL_NEUTRAL_TEXT_PATTERNS) {
    expect(source).not.toMatch(pattern);
  }
}

describe("prompt-editor neutral surface roles", () => {
  const productionSources = walkPromptEditorProductionSources(PROMPT_EDITOR_DIR);

  it.each(
    productionSources.map((path) => [relativePromptEditorPath(path), path]),
  )(
    "%s avoids transitional neutral text class tokens",
    (_label, filePath) => {
      const source = readFileSync(filePath, "utf8");
      expectNoForbiddenTransitionalNeutralText(source);
    },
  );

  it("monaco-prompt-editor uses border-outline on editor shells", () => {
    const source = readPromptEditorSource("monaco-prompt-editor.tsx");

    expect(source).toContain("border-outline");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });

  it("monaco-prompt-editor fallback helper and body copy use text-on-surface-variant", () => {
    const source = readPromptEditorSource("monaco-prompt-editor.tsx");

    expect(source).toContain(
      '<DashboardText className="m-0 text-on-surface-variant" variant="supporting">',
    );
    expect(source).toMatch(
      /<DashboardCode\s+as="pre"\s+className="m-0 whitespace-pre-wrap break-words text-on-surface-variant/,
    );
    expectNoForbiddenTransitionalNeutralText(source);
  });

  it("monaco-guard-selector-editor uses border-outline on editor shells", () => {
    const source = readPromptEditorSource("monaco-guard-selector-editor.tsx");

    expect(source).toContain("border-outline");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });

  it("monaco-guard-selector-editor fallback uses text-on-surface-variant helper and text-on-surface textarea", () => {
    const source = readPromptEditorSource("monaco-guard-selector-editor.tsx");

    expect(source).toContain(
      '<DashboardText className="m-0 text-on-surface-variant" variant="supporting">',
    );
    expect(source).toContain('variant="plain"');
    expectNoForbiddenTransitionalNeutralText(source);
  });

  it("prompt-editor-diagnostics-panel uses bg-surface-container-high on diagnostic list items", () => {
    const source = readPromptEditorSource("prompt-editor-diagnostics-panel.tsx");

    expect(source).toContain("<SurfacePanel");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });

  it("vertical-resizable-width uses outline role utilities on the resize handle", () => {
    const source = readPromptEditorSource("vertical-resizable-width.tsx");

    expect(source).toContain("bg-outline");
    expect(source).toContain("hover:bg-outline-variant");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });
});
