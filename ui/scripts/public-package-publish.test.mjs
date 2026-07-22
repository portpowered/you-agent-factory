import { describe, expect, test } from "vitest";

import {
  assertPublishVersion,
  PUBLIC_PACKAGES,
  patchPublicPackageManifest,
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
  });

  test("accepts stable and immutable development versions", () => {
    expect(assertPublishVersion("1.2.3")).toBe("1.2.3");
    expect(assertPublishVersion("0.0.0-dev.123.abcdef123456")).toBe(
      "0.0.0-dev.123.abcdef123456",
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
});
