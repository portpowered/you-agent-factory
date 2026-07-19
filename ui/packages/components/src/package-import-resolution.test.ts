// @vitest-environment happy-dom

import { beforeAll, describe, expect, it } from "vitest";

import {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  COMPONENTS_PACKAGE_NAME,
} from "@you-agent-factory/components";
import stylesCss from "@you-agent-factory/components/styles.css?inline";
import * as charts from "@you-agent-factory/components/charts";
import * as dataDisplay from "@you-agent-factory/components/data-display";
import * as factoryEmulator from "@you-agent-factory/components/factory-emulator";
import * as feedback from "@you-agent-factory/components/feedback";
import * as forms from "@you-agent-factory/components/forms";
import * as graphs from "@you-agent-factory/components/graphs";
import * as icons from "@you-agent-factory/components/icons";
import * as layout from "@you-agent-factory/components/layout";
import * as navigation from "@you-agent-factory/components/navigation";
import * as overlays from "@you-agent-factory/components/overlays";
import * as primitives from "@you-agent-factory/components/primitives";
import * as recipes from "@you-agent-factory/components/recipes";
import * as testing from "@you-agent-factory/components/testing";
import * as tokens from "@you-agent-factory/components/tokens";
import { cn, COMPONENTS_CATEGORY as utilitiesCategory } from "@you-agent-factory/components/utilities";
import * as utilities from "@you-agent-factory/components/utilities";

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
  "factory-emulator": factoryEmulator,
  graphs,
  recipes,
  icons,
  utilities,
  testing,
  tokens,
} as const;

describe("@you-agent-factory/components package import resolution", () => {
  let documentRoot: HTMLElement;

  beforeAll(async () => {
    documentRoot = await injectCompiledPackageTokenStyles(document);
  });

  it("imports the package root through the configured package path", () => {
    expect(COMPONENTS_PACKAGE_NAME).toBe("@you-agent-factory/components");
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

  it("imports graph primitives from the graphs category surface", () => {
    expect(graphs.COMPONENTS_CATEGORY).toBe("graphs");
    expect(graphs.GraphNodeShell).toBeTypeOf("function");
    expect(graphs.GraphNodeButton).toBeTypeOf("object");
    expect(graphs.GraphEdge).toBeTypeOf("function");
    expect(graphs.GraphViewportSurface).toBeTypeOf("object");
    expect(graphs.GraphNodeHandleBadge).toBeTypeOf("function");
    expect(graphs.buildGraphEdgePathThroughWaypoints).toBeTypeOf("function");
  });

  it("imports cn from the utilities category surface", () => {
    expect(utilitiesCategory).toBe("utilities");
    expect(cn("alpha", false, "beta")).toBe("alpha beta");
    expect(typeof utilities.cn).toBe("function");
  });
});
