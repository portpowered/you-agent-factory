import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const PROMPT_EDITOR_DIR = join(dirname(fileURLToPath(import.meta.url)));

/** Transitional neutral surface/border tokens migrated to Material role utilities. */
const FORBIDDEN_TRANSITIONAL_NEUTRAL_PATTERNS = [
  /\bbg-af-surface-(subtle|raised)\b/,
  /\bbg-af-surface\b/,
  /\bborder-af-border\b/,
  /\bborder-af-border-strong\b/,
  /\bbg-af-border\b/,
  /\bbg-af-border-strong\b/,
  /\bhover:bg-af-border-strong\b/,
];

function readPromptEditorSource(fileName: string): string {
  return readFileSync(join(PROMPT_EDITOR_DIR, fileName), "utf8");
}

function expectNoForbiddenTransitionalNeutralSurfaces(source: string): void {
  for (const pattern of FORBIDDEN_TRANSITIONAL_NEUTRAL_PATTERNS) {
    expect(source).not.toMatch(pattern);
  }
}

describe("prompt-editor neutral surface roles", () => {
  it("monaco-prompt-editor uses border-outline on editor shells", () => {
    const source = readPromptEditorSource("monaco-prompt-editor.tsx");

    expect(source).toContain("border-outline");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });

  it("monaco-guard-selector-editor uses border-outline on editor shells", () => {
    const source = readPromptEditorSource("monaco-guard-selector-editor.tsx");

    expect(source).toContain("border-outline");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });

  it("prompt-editor-diagnostics-panel uses bg-surface-container-high on diagnostic list items", () => {
    const source = readPromptEditorSource("prompt-editor-diagnostics-panel.tsx");

    expect(source).toContain("bg-surface-container-high");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });

  it("vertical-resizable-width uses outline role utilities on the resize handle", () => {
    const source = readPromptEditorSource("vertical-resizable-width.tsx");

    expect(source).toContain("bg-outline");
    expect(source).toContain("hover:bg-outline-variant");
    expectNoForbiddenTransitionalNeutralSurfaces(source);
  });
});
