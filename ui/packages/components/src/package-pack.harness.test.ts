import { access, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

import {
  packAndVerify,
  validatePackedInventory,
} from "../scripts/verify-package-pack.mjs";

const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const temporaryDirectories: string[] = [];

async function temporaryDirectory() {
  const directory = await mkdtemp(path.join(tmpdir(), "components-pack-test-"));
  temporaryDirectories.push(directory);
  return directory;
}

afterEach(async () => {
  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { force: true, recursive: true })),
  );
});

describe("components registry tarball", () => {
  it("builds and verifies the real package-manager inventory", async () => {
    const result = await packAndVerify({
      packageDirectory: packageRoot,
      packDestination: await temporaryDirectory(),
    });

    expect(result.packageName).toBe("@you-agent-factory/components");
    expect(result.files).toContain("package.json");
    expect(result.files).toContain("dist/index.js");
    expect(result.files).toContain("dist/index.d.ts");
    expect(result.files).toContain("dist/styles.css");
    expect(result.files.some((file) => file.startsWith("src/"))).toBe(false);
    expect(result.files.some((file) => file.includes(".stories."))).toBe(false);
    await access(result.tarballPath);
  }, 10_000);

  it("reports missing export targets and transitive style assets", async () => {
    const manifest = {
      exports: {
        ".": {
          import: "./dist/index.js",
          types: "./dist/index.d.ts",
        },
        "./styles.css": "./dist/styles.css",
      },
    };
    const files = ["package.json", "dist/index.js", "dist/styles.css"];

    await expect(
      validatePackedInventory({
        files,
        manifest,
        packageFiles: files,
        readPackedFile: async () =>
          '@import "./tokens.css"; .logo { background: url("./logo.svg"); }',
      }),
    ).rejects.toThrow(
      [
        'missing export target exports["."]["types"]: dist/index.d.ts',
        "missing stylesheet dependency from dist/styles.css: dist/tokens.css",
        "missing stylesheet dependency from dist/styles.css: dist/logo.svg",
      ].join("\n"),
    );
  });

  it("allows stylesheet imports provided by package dependencies", async () => {
    const files = ["package.json", "dist/index.js", "dist/styles.css"];

    await expect(
      validatePackedInventory({
        files,
        manifest: {
          exports: {
            ".": "./dist/index.js",
            "./styles.css": "./dist/styles.css",
          },
        },
        packageFiles: files,
        readPackedFile: async () => '@import "@xyflow/react/dist/style.css";',
      }),
    ).resolves.toEqual([...files].sort());
  });

  it("rejects development-only files admitted to the tarball", async () => {
    const files = [
      "package.json",
      "dist/index.js",
      "src/index.ts",
      "stories/demo.tsx",
    ];

    await expect(
      validatePackedInventory({
        files,
        manifest: { exports: { ".": "./dist/index.js" } },
        packageFiles: files,
        readPackedFile: async () => "",
      }),
    ).rejects.toThrow(
      [
        "development-only files must not be published:",
        "  src/index.ts",
        "  stories/demo.tsx",
      ].join("\n"),
    );
  });
});
