import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  COMPONENTS_PACKAGE_NAME,
} from "youagentfactory/components";
import stylesCss from "youagentfactory/components/styles.css?inline";
import * as charts from "youagentfactory/components/charts";
import * as dataDisplay from "youagentfactory/components/data-display";
import * as feedback from "youagentfactory/components/feedback";
import * as forms from "youagentfactory/components/forms";
import * as graphs from "youagentfactory/components/graphs";
import * as icons from "youagentfactory/components/icons";
import * as layout from "youagentfactory/components/layout";
import * as navigation from "youagentfactory/components/navigation";
import * as overlays from "youagentfactory/components/overlays";
import * as primitives from "youagentfactory/components/primitives";
import * as recipes from "youagentfactory/components/recipes";
import * as testing from "youagentfactory/components/testing";
import * as tokens from "youagentfactory/components/tokens";
import * as utilities from "youagentfactory/components/utilities";

const categoryModules = {
  primitives,
  forms,
  layout,
  feedback,
  "data-display": dataDisplay,
  navigation,
  overlays,
  charts,
  graphs,
  recipes,
  icons,
  utilities,
  testing,
  tokens,
} as const;

const packageRoot = path.dirname(fileURLToPath(import.meta.url));

function readPackageTokenCss(fileName: string): string {
  return readFileSync(
    path.join(packageRoot, "styles", fileName),
    "utf8",
  );
}

describe("youagentfactory/components package import resolution", () => {
  it("imports the package root through the configured package path", () => {
    expect(COMPONENTS_PACKAGE_NAME).toBe("youagentfactory/components");
  });

  it("imports the CSS entrypoint through the configured package path", () => {
    expect(typeof stylesCss).toBe("string");
    expect(stylesCss).not.toMatch(/@import\s+["'].*\/ui\/src\//);

    const stylesEntrypoint = readFileSync(
      path.join(packageRoot, "styles.css"),
      "utf8",
    );
    const palettePresets = readPackageTokenCss("color-palette-presets.css");
    const roleTokens = readPackageTokenCss("color-role-tokens.css");
    const typographyTokens = readPackageTokenCss("typography-role-tokens.css");
    const layoutTokens = readPackageTokenCss("layout-role-tokens.css");

    expect(stylesEntrypoint).toContain(
      '@import "./styles/color-palette-presets.css";',
    );
    expect(palettePresets).toContain("--color-af-foundation-background");
    expect(roleTokens).toContain("--color-primary:");
    expect(typographyTokens).toContain("--text-body-medium:");
    expect(layoutTokens).toContain("--spacing-layout-section:");
    expect(palettePresets).toContain('[data-color-palette="factory-dark"]');
  });

  it.each(COMPONENT_CATEGORY_EXPORT_PATHS)(
    "imports deep category path %s through the configured package path",
    (categoryPath) => {
      const categoryModule = categoryModules[categoryPath];
      expect(categoryModule.COMPONENTS_CATEGORY).toBe(categoryPath);
    },
  );
});
