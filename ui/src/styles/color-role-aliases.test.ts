import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const aliasesSourcePath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "color-role-aliases.css",
);

const ALIAS_SOURCE_PAIRS: ReadonlyArray<
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

describe("color-role-aliases", () => {
  it("maps each transitional af-* token to its Material role source of truth", () => {
    const source = readFileSync(aliasesSourcePath, "utf8");

    for (const [afToken, roleToken] of ALIAS_SOURCE_PAIRS) {
      expect(source).toContain(`${afToken}: var(${roleToken});`);
    }
  });

  it("documents transitional intent in the alias stylesheet header", () => {
    const source = readFileSync(aliasesSourcePath, "utf8");
    expect(source).toContain("Transitional af-*");
    expect(source).toContain("color-role-tokens.css");
  });
});
