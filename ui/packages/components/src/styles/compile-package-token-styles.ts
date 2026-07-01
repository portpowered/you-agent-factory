import path from "node:path";
import { fileURLToPath } from "node:url";

import { compileDashboardStyles } from "../../../../src/test-support/compile-dashboard-styles";

const uiStylesPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "..",
  "..",
  "src",
  "styles.css",
);

function extractBalancedBlock(
  source: string,
  marker: string,
): string {
  const start = source.indexOf(marker);
  if (start === -1) {
    return "";
  }

  const openBraceIndex = source.indexOf("{", start);
  if (openBraceIndex === -1) {
    return "";
  }

  let depth = 0;
  for (let index = openBraceIndex; index < source.length; index += 1) {
    const char = source[index];
    if (char === "{") {
      depth += 1;
    } else if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        return source.slice(start, index + 1);
      }
    }
  }

  return "";
}

function injectCompiledRootRules(
  documentRef: Document,
  compiledCss: string,
): HTMLElement {
  const root =
    documentRef.documentElement ??
    documentRef.appendChild(documentRef.createElement("html"));
  if (!documentRef.head) {
    root.appendChild(documentRef.createElement("head"));
  }

  const rootBlocks = compiledCss.match(/:root[^{]*\{[^}]*\}/g) ?? [];
  const paletteBlocks =
    compiledCss.match(/\[data-color-palette="[^"]+"\][^{]*\{[^}]*\}/g) ?? [];
  const themeLayer = extractBalancedBlock(compiledCss, "@layer theme");
  const style = documentRef.createElement("style");
  style.textContent = [
    themeLayer,
    ...rootBlocks,
    ...paletteBlocks,
  ].join("\n");
  documentRef.head?.appendChild(style);
  return root;
}

export async function injectCompiledPackageTokenStyles(
  documentRef: Document,
): Promise<HTMLElement> {
  const compiledCss = await compileDashboardStyles(uiStylesPath);
  return injectCompiledRootRules(documentRef, compiledCss);
}

export function readDocumentCssVariable(
  root: HTMLElement,
  name: string,
): string {
  return getComputedStyle(root).getPropertyValue(name).trim();
}
