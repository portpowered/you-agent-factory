import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import packageJson from "../package.json";

const packageDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

describe("youagentfactory/components styles.css entrypoint", () => {
  it("exports a package-owned CSS file at ./styles.css", () => {
    const exportPath = packageJson.exports["./styles.css"];
    expect(exportPath).toBe("./src/styles.css");

    const resolvedPath = path.resolve(packageDir, exportPath);
    expect(resolvedPath.startsWith(path.join(packageDir, "src"))).toBe(true);
  });

  it("resolves to an existing CSS file that does not depend on dashboard modules", async () => {
    const exportPath = packageJson.exports["./styles.css"] as string;
    const resolvedPath = path.resolve(packageDir, exportPath);
    const cssSource = await readFile(resolvedPath, "utf8");

    expect(resolvedPath.endsWith(".css")).toBe(true);
    expect(cssSource.length).toBeGreaterThan(0);
    expect(cssSource).not.toMatch(/@import\s+["'].*\/ui\/src\//);
    expect(cssSource).not.toMatch(/url\(["']?.*\/ui\/src\//);
  });
});
