import { access } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import packageJson from "../package.json";
import { COMPONENT_CATEGORY_EXPORT_PATHS } from "./category-paths";

const packageDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

type PackageExportEntry =
  | string
  | {
      types?: string;
      default?: string;
    };

function getCategoryExportEntry(
  exportPath: string,
): PackageExportEntry | undefined {
  return packageJson.exports[exportPath as keyof typeof packageJson.exports];
}

function resolveCategorySourcePath(exportPath: string): string {
  const exportEntry = getCategoryExportEntry(`./${exportPath}`);
  expect(exportEntry).toBeDefined();

  const sourcePath =
    typeof exportEntry === "string"
      ? exportEntry
      : (exportEntry?.default ?? exportEntry?.types);
  expect(sourcePath).toBeDefined();

  return path.resolve(packageDir, sourcePath as string);
}

describe("youagentfactory/components deep category exports", () => {
  it("declares stable export paths for all planned categories", () => {
    for (const categoryPath of COMPONENT_CATEGORY_EXPORT_PATHS) {
      expect(getCategoryExportEntry(`./${categoryPath}`)).toBeDefined();
    }
  });

  it.each(COMPONENT_CATEGORY_EXPORT_PATHS)(
    "resolves ./%s to a package-owned source entrypoint",
    async (categoryPath) => {
      const resolvedPath = resolveCategorySourcePath(categoryPath);

      expect(resolvedPath.startsWith(path.join(packageDir, "src"))).toBe(true);
      await expect(access(resolvedPath)).resolves.toBeUndefined();
    },
  );
});
