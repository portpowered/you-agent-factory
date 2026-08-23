// @vitest-environment happy-dom

import { beforeAll, describe, expect, it } from "vitest";

import {
  injectCompiledPackageTokenStyles,
  readDocumentCssVariable,
} from "./compile-package-token-styles";

let documentRoot: HTMLElement;

beforeAll(async () => {
  documentRoot = await injectCompiledPackageTokenStyles(document);
});

function normalizeCssValue(value: string): string {
  return value
    .replace(/\s+/g, " ")
    .replace(/\(\s+/g, "(")
    .replace(/\s+\)/g, ")")
    .trim();
}

describe("@you-agent-factory/components token styles entrypoint", () => {
  it("exposes role tokens on the document root after importing package styles", () => {
    expect(
      readDocumentCssVariable(documentRoot, "--color-primary"),
    ).toBeTruthy();
    expect(readDocumentCssVariable(documentRoot, "--color-on-surface")).toBe(
      "#f7f2e8",
    );
  });

  it("exposes typography and layout tokens from the package styles entrypoint", () => {
    expect(
      readDocumentCssVariable(documentRoot, "--text-title-large"),
    ).toBeTruthy();
    expect(
      readDocumentCssVariable(documentRoot, "--text-body-medium"),
    ).toBeTruthy();
    expect(
      readDocumentCssVariable(documentRoot, "--text-label-medium"),
    ).toBeTruthy();
    expect(
      readDocumentCssVariable(documentRoot, "--color-af-foundation-surface"),
    ).toBeTruthy();
  });

  it("switches palette foundation background when data-color-palette changes", () => {
    documentRoot.dataset.colorPalette = "factory-dark";
    const factoryDarkBackground = readDocumentCssVariable(
      documentRoot,
      "--color-af-foundation-background",
    );
    expect(factoryDarkBackground.toLowerCase()).toBe("#050b10");

    documentRoot.dataset.colorPalette = "factory-light";
    const factoryLightBackground = readDocumentCssVariable(
      documentRoot,
      "--color-af-foundation-background",
    );
    expect(factoryLightBackground.toLowerCase()).toBe("#f4f1e8");
    expect(factoryLightBackground).not.toBe(factoryDarkBackground);
  });

  it("defines the dialog scrim role from the active palette background", () => {
    documentRoot.dataset.colorPalette = "factory-dark";
    const factoryDarkScrim = readDocumentCssVariable(
      documentRoot,
      "--color-scrim",
    );
    documentRoot.dataset.colorPalette = "factory-light";
    const factoryLightScrim = readDocumentCssVariable(
      documentRoot,
      "--color-scrim",
    );

    expect(normalizeCssValue(factoryDarkScrim)).toBe(
      "rgb(from #050b10 r g b / 0.7)",
    );
    expect(normalizeCssValue(factoryLightScrim)).toBe(
      "rgb(from #f4f1e8 r g b / 0.7)",
    );
    expect(factoryLightScrim).not.toBe(factoryDarkScrim);
  });
});

describe("@you-agent-factory/components graph emphasis roles", () => {
  it("exposes palette-derived graph emphasis roles", () => {
    documentRoot.dataset.colorPalette = "factory-dark";

    const selectedRing = readDocumentCssVariable(
      documentRoot,
      "--color-af-graph-selected-ring",
    );
    const errorRing = readDocumentCssVariable(
      documentRoot,
      "--color-af-graph-error-ring",
    );
    const selectedShadow = readDocumentCssVariable(
      documentRoot,
      "--shadow-af-graph-selected",
    );
    const errorShadow = readDocumentCssVariable(
      documentRoot,
      "--shadow-af-graph-error",
    );

    expect(normalizeCssValue(selectedRing)).toBe(
      "rgb(from #f5c76f r g b / 0.3)",
    );
    expect(normalizeCssValue(errorRing)).toBe("rgb(from #f05f5f r g b / 0.4)");
    expect(normalizeCssValue(selectedShadow)).toContain(
      "0 0 0 1px rgb(from #f5c76f r g b / 0.28)",
    );
    expect(normalizeCssValue(selectedShadow)).toContain(
      "0 0 0 4px rgb(from #f5c76f r g b / 0.08)",
    );
    expect(normalizeCssValue(errorShadow)).toContain("0 0 0 3px");

    documentRoot.dataset.colorPalette = "factory-light";
    const factoryLightSelectedRing = readDocumentCssVariable(
      documentRoot,
      "--color-af-graph-selected-ring",
    );
    const factoryLightErrorRing = readDocumentCssVariable(
      documentRoot,
      "--color-af-graph-error-ring",
    );

    expect(normalizeCssValue(factoryLightSelectedRing)).toBe(
      "rgb(from #f5c76f r g b / 0.3)",
    );
    expect(normalizeCssValue(factoryLightErrorRing)).toBe(
      "rgb(from #d94848 r g b / 0.4)",
    );
  });
});
