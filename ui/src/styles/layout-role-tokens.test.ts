import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { DASHBOARD_LAYOUT_CONTRACT } from "../components/ui/dashboard-layout";

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
const layoutTokensPath = path.join(packageStylesDir, "layout-role-tokens.css");
const stylesSourcePath = path.join(stylesDir, "..", "styles.css");
const dialogSourcePath = path.join(
  stylesDir,
  "..",
  "components",
  "ui",
  "dialog.tsx",
);
const layoutPrimitivesPath = path.join(
  stylesDir,
  "..",
  "components",
  "ui",
  "layout-primitives.tsx",
);
const dashboardLayoutPath = path.join(
  stylesDir,
  "..",
  "components",
  "ui",
  "dashboard-layout.ts",
);

const LAYOUT_SPACING_ROLES = [
  "tight",
  "element",
  "group",
  "block",
  "section",
  "page",
] as const;

describe("layout-role-tokens (US-007)", () => {
  const layoutSource = readFileSync(layoutTokensPath, "utf8");
  const stylesSource = readFileSync(stylesSourcePath, "utf8");
  const dialogSource = readFileSync(dialogSourcePath, "utf8");
  const primitivesSource = readFileSync(layoutPrimitivesPath, "utf8");
  const dashboardLayoutSource = readFileSync(dashboardLayoutPath, "utf8");

  it("defines semantic layout spacing roles on a 4px grid", () => {
    for (const role of LAYOUT_SPACING_ROLES) {
      expect(layoutSource).toContain(`--spacing-layout-${role}:`);
    }
    expect(layoutSource).toContain("--spacing-layout-inset-dialog:");
    expect(layoutSource).toContain("--spacing-layout-inset-card:");
  });

  it("imports layout tokens from the component package styles entrypoint", () => {
    expect(stylesSource).toContain(
      '@import "@you-agent-factory/components/styles.css";',
    );
  });

  it("maps layout contract entries to primitive exports", () => {
    for (const entry of DASHBOARD_LAYOUT_CONTRACT) {
      expect(entry.className).toContain(entry.gapUtility);
      expect(dashboardLayoutSource).toContain(entry.className);
    }
    expect(primitivesSource).toContain("LAYOUT_SECTION_STACK_CLASS");
    expect(primitivesSource).toContain("LAYOUT_DIALOG_BODY_CLASS");
  });

  it("wires shared Dialog chrome to layout spacing roles", () => {
    expect(dialogSource).toContain("LAYOUT_DIALOG_CONTENT_SHELL_CLASS");
    expect(dialogSource).toContain("LAYOUT_FORM_GROUP_CLASS");
    expect(dialogSource).toContain("gap-layout-element");
  });
});
