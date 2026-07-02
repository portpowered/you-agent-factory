// @vitest-environment happy-dom

import { beforeAll, describe, expect, it } from "vitest";

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
import { cn, COMPONENTS_CATEGORY as utilitiesCategory } from "youagentfactory/components/utilities";
import * as utilities from "youagentfactory/components/utilities";

import {
  injectCompiledPackageTokenStyles,
  readDocumentCssVariable,
} from "./styles/compile-package-token-styles";

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

describe("youagentfactory/components package import resolution", () => {
  let documentRoot: HTMLElement;

  beforeAll(async () => {
    documentRoot = await injectCompiledPackageTokenStyles(document);
  });

  it("imports the package root through the configured package path", () => {
    expect(COMPONENTS_PACKAGE_NAME).toBe("youagentfactory/components");
  });

  it("imports the CSS entrypoint through the configured package path", () => {
    expect(typeof stylesCss).toBe("string");
    expect(stylesCss).not.toMatch(/@import\s+["'].*\/ui\/src\//);
    expect(readDocumentCssVariable(documentRoot, "--color-primary")).toBeTruthy();
    expect(readDocumentCssVariable(documentRoot, "--text-body-medium")).toBeTruthy();
    expect(readDocumentCssVariable(documentRoot, "--text-title-large")).toBeTruthy();
    expect(readDocumentCssVariable(documentRoot, "--color-af-foundation-background")).toBeTruthy();
  });

  it.each(COMPONENT_CATEGORY_EXPORT_PATHS)(
    "imports deep category path %s through the configured package path",
    (categoryPath) => {
      const categoryModule = categoryModules[categoryPath];
      expect(categoryModule.COMPONENTS_CATEGORY).toBe(categoryPath);
    },
  );

  it("imports cn from the utilities category surface", () => {
    expect(utilitiesCategory).toBe("utilities");
    expect(cn("alpha", false, "beta")).toBe("alpha beta");
    expect(typeof utilities.cn).toBe("function");
  });
});
