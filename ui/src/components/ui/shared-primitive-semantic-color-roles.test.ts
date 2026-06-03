import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const UI_COMPONENTS_DIR = join(dirname(fileURLToPath(import.meta.url)));

function readComponentSource(fileName: string): string {
  return readFileSync(join(UI_COMPONENTS_DIR, fileName), "utf8");
}

describe("shared primitive semantic color roles", () => {
  it("maps standard list accent rows to primary accent tokens, not semantic info", () => {
    const source = readComponentSource("standard-list-selection.tsx");

    expect(source).toContain("STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS");
    expect(source).toContain("border-af-accent-border bg-af-accent-surface");
    expect(source).not.toContain("STANDARD_LIST_SELECTION_ROW_INFO_CLASS");
    expect(source).not.toContain("border-af-info-border");
    expect(source).not.toContain('"info"');
  });

  it("reserves dashboard status pill warning and info classes for semantic tones only", () => {
    const source = readComponentSource("dashboard-status-pill.tsx");

    expect(source).toMatch(/warning:\s*\n?\s*"border-af-warning-border/);
    expect(source).toMatch(/info:\s*\n?\s*"border-af-info-border/);
    expect(source).toMatch(/active:\s*\n?\s*"border-af-accent-border/);
  });

  it("keeps the default button tone on primary accent tokens, not warning or success", () => {
    const source = readComponentSource("button.tsx");

    expect(source).toContain("border-af-accent bg-af-accent");
    expect(source).not.toContain("af-warning");
    expect(source).not.toContain("af-success");
  });
});
