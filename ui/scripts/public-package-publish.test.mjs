import { describe, expect, test } from "vitest";

import {
  assertPackedExportTargets,
  assertPackedRequiredFiles,
  assertPublishVersion,
  PUBLIC_PACKAGES,
  patchPublicPackageManifest,
  RELEASE_PUBLIC_PACKAGES,
} from "./public-package-publish.mjs";

describe("public package publishing", () => {
  test("publishes the canonical package family in dependency order", () => {
    expect(PUBLIC_PACKAGES.map(({ name }) => name)).toEqual([
      "@you-agent-factory/client",
      "@you-agent-factory/factory-replay",
      "@you-agent-factory/factory-emulator",
      "@you-agent-factory/components",
      "@you-agent-factory/factory-visualizers",
    ]);
    expect(RELEASE_PUBLIC_PACKAGES.map(({ name }) => name)).toEqual([
      "@you-agent-factory/api",
      "@you-agent-factory/packaged-factories",
      ...PUBLIC_PACKAGES.map(({ name }) => name),
    ]);
  });

  test("accepts stable and immutable development versions", () => {
    expect(assertPublishVersion("1.2.3")).toBe("1.2.3");
    expect(assertPublishVersion("0.0.2-dev.123.abcdef123456")).toBe(
      "0.0.2-dev.123.abcdef123456",
    );
    expect(() => assertPublishVersion("v1.2.3")).toThrow(
      "Invalid public package version",
    );
  });

  test("aligns every internal package dependency with the candidate version", () => {
    const manifest = {
      name: "@you-agent-factory/factory-visualizers",
      version: "0.0.0",
      dependencies: {
        "@you-agent-factory/components": "0.0.0",
        "@xyflow/react": "^12.0.0",
      },
      peerDependencies: {
        "@you-agent-factory/client": "0.0.0",
        react: "^19.0.0",
      },
      devDependencies: {
        "@you-agent-factory/factory-replay": "file:../factory-replay",
      },
    };
    expect(patchPublicPackageManifest(manifest, "2.0.0")).toEqual({
      ...manifest,
      version: "2.0.0",
      dependencies: {
        "@you-agent-factory/components": "2.0.0",
        "@xyflow/react": "^12.0.0",
      },
      peerDependencies: {
        "@you-agent-factory/client": "2.0.0",
        react: "^19.0.0",
      },
      devDependencies: {
        "@you-agent-factory/factory-replay": "2.0.0",
      },
    });
    expect(manifest.version).toBe("0.0.0");
  });

  test("rejects release candidates with missing export targets", () => {
    const exports = {
      ".": {
        types: "./dist/index.d.ts",
        import: "./dist/index.js",
      },
      "./styles.css": "./dist/styles.css",
    };
    const completeFiles = [
      { path: "dist/index.d.ts" },
      { path: "dist/index.js" },
      { path: "dist/styles.css" },
    ];

    expect(() =>
      assertPackedExportTargets(
        "@you-agent-factory/factory-visualizers",
        exports,
        completeFiles,
      ),
    ).not.toThrow();
    expect(() =>
      assertPackedExportTargets(
        "@you-agent-factory/factory-visualizers",
        exports,
        completeFiles.filter(({ path }) => path !== "dist/index.js"),
      ),
    ).toThrow(
      "@you-agent-factory/factory-visualizers candidate omits export target dist/index.js",
    );
  });

  test("accepts populated wildcard export targets", () => {
    const exports = { "./joined/*": "./generated/joined/*" };

    expect(() =>
      assertPackedExportTargets("@you-agent-factory/api", exports, [
        { path: "generated/joined/contracts/manifest.schema.json" },
      ]),
    ).not.toThrow();
    expect(() =>
      assertPackedExportTargets("@you-agent-factory/api", exports, [
        { path: "generated/manifest.json" },
      ]),
    ).toThrow(
      "@you-agent-factory/api candidate omits export target generated/joined/*",
    );
  });

  test("isolates the packaged factories legacy deep-path contract", () => {
    expect(() =>
      assertPackedRequiredFiles(
        "@you-agent-factory/packaged-factories",
        ["factories/goal/factory.json"],
        [{ path: "factories/goal/factory.json" }],
      ),
    ).not.toThrow();
    expect(() =>
      assertPackedRequiredFiles(
        "@you-agent-factory/packaged-factories",
        ["factories/goal/factory.json"],
        [{ path: "factories/review/factory.json" }],
      ),
    ).toThrow(
      "@you-agent-factory/packaged-factories candidate omits factories/goal/factory.json",
    );
  });
});
