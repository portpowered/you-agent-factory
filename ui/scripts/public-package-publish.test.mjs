import { access, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, expect, test } from "vitest";
import {
  FRONTEND_ONLY_CANDIDATE_SCOPE,
  TAGGED_RELEASE_CANDIDATE_SCOPE,
} from "../../scripts/public-package-set.mjs";
import {
  assertFrontendCandidateEvidence,
  assertPublishVersion,
  PUBLIC_PACKAGES,
  packCandidate,
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

  test("candidate packing suppresses lifecycle script side effects", async () => {
    const root = await mkdtemp(
      path.join(tmpdir(), "you-package-lifecycle-fixture-"),
    );
    const stagedDirectory = path.join(root, "package");
    const outputDirectory = path.join(root, "candidate");
    const sentinel = path.join(root, "lifecycle-ran");
    try {
      await Promise.all([mkdir(stagedDirectory), mkdir(outputDirectory)]);
      await Promise.all([
        writeFile(
          path.join(stagedDirectory, "package.json"),
          `${JSON.stringify({
            name: "@you-agent-factory/lifecycle-fixture",
            version: "1.0.0",
            scripts: { prepack: "node create-sentinel.mjs" },
          })}\n`,
        ),
        writeFile(
          path.join(stagedDirectory, "create-sentinel.mjs"),
          `import { writeFileSync } from "node:fs"; writeFileSync(${JSON.stringify(sentinel)}, "ran");\n`,
        ),
      ]);
      const { stdout } = await packCandidate({
        stagedDirectory,
        outputDirectory,
      });
      expect(JSON.parse(stdout)[0].name).toBe(
        "@you-agent-factory/lifecycle-fixture",
      );
      await expect(access(sentinel)).rejects.toMatchObject({ code: "ENOENT" });
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });

  test("accepts only a complete frontend-only development candidate", () => {
    const evidence = {
      scope: FRONTEND_ONLY_CANDIDATE_SCOPE,
      version: "0.0.0-dev.123.abcdef123456",
      packages: PUBLIC_PACKAGES.map(({ name }) => ({
        name,
        version: "0.0.0-dev.123.abcdef123456",
      })),
    };
    expect(assertFrontendCandidateEvidence(evidence)).toBe(evidence);
    expect(() =>
      assertFrontendCandidateEvidence({
        ...evidence,
        scope: TAGGED_RELEASE_CANDIDATE_SCOPE,
      }),
    ).toThrow("expected frontend-only candidate scope");
  });
});
