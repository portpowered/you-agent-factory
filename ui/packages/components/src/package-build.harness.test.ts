import { execFile } from "node:child_process";
import { access, readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";
import { describe, expect, it } from "vitest";

import { COMPONENT_CATEGORY_EXPORT_PATHS } from "./category-paths";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const distRoot = path.join(packageRoot, "dist");
const execFileAsync = promisify(execFile);

interface PackageExportTarget {
  default: string;
  import: string;
  types: string;
}

interface PackageManifest {
  dependencies: Record<string, string>;
  description: string;
  exports: Record<string, PackageExportTarget | string>;
  files: string[];
  license: string;
  peerDependencies: Record<string, string>;
  private?: boolean;
  repository: { directory: string; type: string; url: string };
  publishConfig: { access: string };
  sideEffects: string[];
}

async function readPackageManifest(): Promise<PackageManifest> {
  return JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
}

async function listFilesRecursively(directory: string): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const nestedFiles = await Promise.all(
    entries.map((entry) => {
      const entryPath = path.join(directory, entry.name);
      return entry.isDirectory() ? listFilesRecursively(entryPath) : entryPath;
    }),
  );
  return nestedFiles.flat();
}

async function importBuiltEntry(relativePath: string) {
  return import(pathToFileURL(path.join(distRoot, relativePath)).href);
}

describe("components package production build", () => {
  it("emits ESM and declarations for the root and every category", async () => {
    const entryPaths = [
      "index",
      ...COMPONENT_CATEGORY_EXPORT_PATHS.map(
        (categoryPath) => `${categoryPath}/index`,
      ),
    ];

    await Promise.all(
      entryPaths.flatMap((entryPath) => [
        access(path.join(distRoot, `${entryPath}.js`)),
        access(path.join(distRoot, `${entryPath}.d.ts`)),
      ]),
    );
  });

  it("loads representative exports from emitted JavaScript", async () => {
    const [root, primitives, utilities, charts, graphs] = await Promise.all([
      importBuiltEntry("index.js"),
      importBuiltEntry("primitives/index.js"),
      importBuiltEntry("utilities/index.js"),
      importBuiltEntry("charts/index.js"),
      importBuiltEntry("graphs/index.js"),
    ]);

    expect(root.COMPONENTS_PACKAGE_NAME).toBe("@you-agent-factory/components");
    expect(primitives.Button).toBeDefined();
    expect(utilities.cn("alpha", false, "beta")).toBe("alpha beta");
    expect(charts.ChartContainer).toBeDefined();
    expect(graphs.GraphNodeShell).toBeDefined();
  });

  it("copies a self-contained stylesheet and asset tree", async () => {
    const pendingStylesheets = [path.join(distRoot, "styles.css")];
    const visitedStylesheets = new Set<string>();

    while (pendingStylesheets.length > 0) {
      const stylesheetPath = pendingStylesheets.pop();
      if (!stylesheetPath || visitedStylesheets.has(stylesheetPath)) continue;
      visitedStylesheets.add(stylesheetPath);

      const styles = await readFile(stylesheetPath, "utf8");
      const referencedPaths = [
        ...styles.matchAll(/@import\s+["'](.+?)["']/g),
        ...styles.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/g),
      ].map((match) => match[1]);

      for (const referencedPath of referencedPaths) {
        if (/^(?:[a-z]+:|data:|#)/i.test(referencedPath)) continue;
        const resolvedPath = path.resolve(
          path.dirname(stylesheetPath),
          referencedPath.split(/[?#]/, 1)[0],
        );
        await access(resolvedPath);
        if (path.extname(resolvedPath) === ".css") {
          pendingStylesheets.push(resolvedPath);
        }
      }
    }

    expect(visitedStylesheets.size).toBeGreaterThan(1);
  });

  it("publishes intentional runtime, type, and style entrypoints", async () => {
    const manifest = await readPackageManifest();
    const expectedExportPaths = [
      ".",
      "./styles.css",
      ...COMPONENT_CATEGORY_EXPORT_PATHS.map(
        (categoryPath) => `./${categoryPath}`,
      ),
    ];

    expect(Object.keys(manifest.exports)).toEqual(expectedExportPaths);
    expect(manifest.exports["./styles.css"]).toBe("./dist/styles.css");

    for (const exportPath of expectedExportPaths.filter(
      (candidate) => candidate !== "./styles.css",
    )) {
      const target = manifest.exports[exportPath] as PackageExportTarget;
      expect(target.import).toMatch(/^\.\/dist\/.+\.js$/);
      expect(target.default).toBe(target.import);
      expect(target.types).toMatch(/^\.\/dist\/.+\.d\.ts$/);
      await Promise.all([
        access(path.join(packageRoot, target.import)),
        access(path.join(packageRoot, target.types)),
      ]);
    }
  });

  it("resolves supported self-references and rejects undeclared deep imports", async () => {
    const supportedSpecifiers = [
      "@you-agent-factory/components",
      ...COMPONENT_CATEGORY_EXPORT_PATHS.map(
        (categoryPath) => `@you-agent-factory/components/${categoryPath}`,
      ),
    ];

    const { stdout } = await execFileAsync(
      process.execPath,
      [
        "--input-type=module",
        "--eval",
        `
          const supported = ${JSON.stringify(supportedSpecifiers)};
          for (const specifier of supported) {
            const resolved = import.meta.resolve(specifier);
            if (!resolved.includes("/dist/")) {
              throw new Error(specifier + " resolved outside dist: " + resolved);
            }
          }
          try {
            import.meta.resolve("@you-agent-factory/components/src/index.ts");
            throw new Error("undeclared deep import unexpectedly resolved");
          } catch (error) {
            if (error.code !== "ERR_PACKAGE_PATH_NOT_EXPORTED") throw error;
          }
          process.stdout.write("resolved");
        `,
      ],
      { cwd: packageRoot },
    );

    expect(stdout).toBe("resolved");
  });

  it("declares every external runtime import and keeps React host-provided", async () => {
    const manifest = await readPackageManifest();
    const runtimePackages = new Set<string>();
    const packageNameFromSpecifier = (specifier: string) =>
      specifier.startsWith("@")
        ? specifier.split("/").slice(0, 2).join("/")
        : specifier.split("/", 1)[0];

    for (const filePath of await listFilesRecursively(distRoot)) {
      if (path.extname(filePath) !== ".js") continue;
      const source = await readFile(filePath, "utf8");
      for (const match of source.matchAll(
        /(?:from\s+|import\s+)["']([^./][^"']*)["']/g,
      )) {
        runtimePackages.add(packageNameFromSpecifier(match[1]));
      }
    }

    expect(manifest.peerDependencies).toEqual({
      react: "^19.0.0",
      "react-dom": "^19.0.0",
    });
    expect(manifest.dependencies).not.toHaveProperty("react");
    expect(manifest.dependencies).not.toHaveProperty("react-dom");
    for (const runtimePackage of runtimePackages) {
      expect(
        runtimePackage in manifest.dependencies ||
          runtimePackage in manifest.peerDependencies,
        `${runtimePackage} must be declared as a dependency or peer dependency`,
      ).toBe(true);
    }
  });

  it("declares public package metadata and CSS side effects", async () => {
    const manifest = await readPackageManifest();

    expect(manifest.license).toBe("MIT");
    expect(manifest.description).not.toHaveLength(0);
    expect(manifest.private).toBeUndefined();
    expect(manifest.repository).toEqual({
      type: "git",
      url: "git+https://github.com/portpowered/you-agent-factory.git",
      directory: "ui/packages/components",
    });
    expect(manifest.publishConfig).toEqual({ access: "public" });
    expect(manifest.files).toEqual(["dist", "LICENSE.md", "README.md"]);
    expect(manifest.sideEffects).toContain("**/*.css");
    await access(path.join(packageRoot, "LICENSE.md"));
  });
});
