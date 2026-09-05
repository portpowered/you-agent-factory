import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { compileDashboardStyles } from "../src/test-support/compile-dashboard-styles";
import { COLOR_PALETTE_IDS } from "../src/theme/color-palette";
import {
  generateColorPaletteTokens,
  renderColorPaletteTokenModule,
  renderColorPaletteTokens,
} from "./color-palette-token-generator";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const stylesSourcePath = path.join(uiRoot, "src", "styles.css");
const outputPath = path.join(uiRoot, "src", "theme", "color-palette-tokens.ts");
const generatedDirectory = path.join(uiRoot, "src", "theme", "generated");
const checkOnly = process.argv.includes("--check");

const compiledCss = await compileDashboardStyles(stylesSourcePath);
const graph = generateColorPaletteTokens(compiledCss);
const expectedIndex = renderColorPaletteTokens();
const expectedModules = COLOR_PALETTE_IDS.map((paletteId) => ({
  path: path.join(generatedDirectory, `color-palette-tokens-${paletteId}.ts`),
  source: renderColorPaletteTokenModule(paletteId, graph[paletteId]),
}));

if (checkOnly) {
  const currentIndex = await readOptionalFile(outputPath);
  const currentModules = await Promise.all(
    expectedModules.map(async ({ path: modulePath, source }) => ({
      current: await readOptionalFile(modulePath),
      source,
    })),
  );

  if (
    currentIndex !== expectedIndex ||
    currentModules.some(({ current, source }) => current !== source)
  ) {
    console.error(
      "Generated color palette tokens are stale. Run `bun run generate:color-palette-tokens` in ui.",
    );
    process.exitCode = 1;
  }
} else {
  await mkdir(generatedDirectory, { recursive: true });
  await writeFile(outputPath, expectedIndex, "utf8");
  await Promise.all(
    expectedModules.map(({ path: modulePath, source }) =>
      writeFile(modulePath, source, "utf8"),
    ),
  );
  console.log(`Generated ${path.relative(uiRoot, outputPath)}`);
}

async function readOptionalFile(filePath: string): Promise<string | undefined> {
  return readFile(filePath, "utf8").catch((error: unknown) => {
    if (error instanceof Error && "code" in error && error.code === "ENOENT") {
      return undefined;
    }
    throw error;
  });
}
