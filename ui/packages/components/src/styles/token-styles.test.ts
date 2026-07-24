// @vitest-environment happy-dom

import { beforeAll, describe, expect, it } from "vitest";

import {
  injectCompiledPackageTokenStyles,
  readDocumentCssVariable,
} from "./compile-package-token-styles";

describe("@you-agent-factory/components token styles entrypoint", () => {
  let documentRoot: HTMLElement;

  beforeAll(async () => {
    documentRoot = await injectCompiledPackageTokenStyles(document);
  });

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
});
