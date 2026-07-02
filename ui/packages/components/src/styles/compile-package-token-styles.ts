import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { compile, type Resolver } from "@tailwindcss/node";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
);
const packageStylesPath = path.join(packageRoot, "src", "styles.css");
const packageTokenFixturePath = path.join(
  packageRoot,
  "src",
  "styles",
  "package-token-styles-fixture.css",
);

function createPackageCssResolver(): Resolver {
  return async (id) => {
    if (id === "youagentfactory/components/styles.css") {
      return packageStylesPath;
    }
    return undefined;
  };
}

function extractBalancedBlock(source: string, marker: string): string {
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
  style.textContent = [themeLayer, ...rootBlocks, ...paletteBlocks].join("\n");
  documentRef.head?.appendChild(style);
  return root;
}

export async function compilePackageTokenFixtureStyles(): Promise<string> {
  const source = readFileSync(packageTokenFixturePath, "utf8");
  const compiled = await compile(source, {
    base: path.dirname(packageTokenFixturePath),
    from: packageTokenFixturePath,
    onDependency: () => {},
    customCssResolver: createPackageCssResolver(),
  });
  return compiled.build([]);
}

export async function injectCompiledPackageTokenStyles(
  documentRef: Document,
): Promise<HTMLElement> {
  const compiledCss = await compilePackageTokenFixtureStyles();
  return injectCompiledRootRules(documentRef, compiledCss);
}

export function readDocumentCssVariable(
  root: HTMLElement,
  name: string,
): string {
  return getComputedStyle(root).getPropertyValue(name).trim();
}
