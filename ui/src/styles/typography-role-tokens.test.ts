import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import {
  DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES,
  DASHBOARD_TYPOGRAPHY_CONTRACT,
} from "../components/ui/dashboard-typography";

const stylesDir = path.dirname(fileURLToPath(import.meta.url));
const typographyTokensPath = path.join(stylesDir, "typography-role-tokens.css");
const typographyUtilitiesPath = path.join(
  stylesDir,
  "typography-role-utilities.css",
);
const textColorTokensPath = path.join(stylesDir, "text-color-role-tokens.css");
const stylesSourcePath = path.join(stylesDir, "..", "styles.css");
const roleTokensSourcePath = path.join(stylesDir, "color-role-tokens.css");

const MATERIAL_SCALE_FAMILIES = [
  "display",
  "headline",
  "title",
  "body",
  "label",
] as const;

const PRODUCT_USED_VARIANTS = ["large", "medium", "small"] as const;

describe("typography-role-tokens (US-006)", () => {
  const typographySource = readFileSync(typographyTokensPath, "utf8");
  const utilitiesSource = readFileSync(typographyUtilitiesPath, "utf8");
  const textColorSource = readFileSync(textColorTokensPath, "utf8");
  const stylesSource = readFileSync(stylesSourcePath, "utf8");
  const roleTokensSource = readFileSync(roleTokensSourcePath, "utf8");

  it("defines Material 3 scale families with large/medium/small variants", () => {
    for (const family of MATERIAL_SCALE_FAMILIES) {
      for (const variant of PRODUCT_USED_VARIANTS) {
        expect(typographySource).toContain(`--text-${family}-${variant}:`);
      }
    }
    expect(typographySource).toContain("--text-code-medium:");
    expect(typographySource).toContain("--text-code-small:");
  });

  it("exposes composed type utilities for dashboard mappings", () => {
    for (const entry of DASHBOARD_TYPOGRAPHY_CONTRACT) {
      expect(utilitiesSource).toContain(`.${entry.typeUtilityClass}`);
    }
    for (const entry of DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES) {
      expect(utilitiesSource).toContain(`.${entry.typeUtilityClass}`);
    }
  });

  it("defines explicit text color roles including inverse, disabled, and code", () => {
    expect(textColorSource).toContain("--color-on-surface:");
    expect(textColorSource).toContain("--color-on-surface-variant:");
    expect(textColorSource).toContain("--color-on-surface-subtle:");
    expect(textColorSource).toContain("--color-on-surface-disabled:");
    expect(textColorSource).toContain("--color-on-inverse:");
    expect(textColorSource).toContain("--color-code:");
  });

  it("maps dashboard semantic classes through scale utilities and text color roles", () => {
    expect(stylesSource).toContain(
      ".af-dashboard-page-heading {\n    @apply font-display text-display-medium text-on-surface;",
    );
    expect(stylesSource).toContain(
      ".af-dashboard-body-text {\n    @apply text-body-medium text-on-surface-variant;",
    );
    expect(stylesSource).toContain(
      ".af-dashboard-body-code {\n    @apply font-mono text-code-medium text-code;",
    );
  });

  it("maps product af-* text tokens to text color roles in color-role-tokens.css", () => {
    expect(roleTokensSource).toContain(
      "--color-af-text-disabled: var(--color-on-surface-disabled);",
    );
    expect(roleTokensSource).toContain(
      "--color-af-text-inverse: var(--color-on-inverse);",
    );
    expect(roleTokensSource).toContain(
      "--color-af-code-ink: var(--color-code);",
    );
  });
});
