import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, describe, expect, it } from "vitest";

import {
  inspectPublicPackageBoundaries,
  publicPackagePolicies,
} from "./check-public-package-boundaries.mjs";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function createPackageFamily(packagesRoot, dependencyOverrides = {}) {
  for (const [packageKey, policy] of Object.entries(publicPackagePolicies)) {
    const packageRoot = path.join(packagesRoot, packageKey);
    await mkdir(path.join(packageRoot, "src"), { recursive: true });
    await writeFile(
      path.join(packageRoot, "package.json"),
      `${JSON.stringify(
        {
          name: policy.packageName,
          type: "module",
          dependencies: dependencyOverrides[packageKey] ?? {},
        },
        null,
        2,
      )}\n`,
    );
    await writeFile(path.join(packageRoot, "src", "index.ts"), "export {};\n");
  }
}

describe("public package dependency boundaries", () => {
  const tempRoots = [];

  afterEach(async () => {
    await Promise.all(
      tempRoots
        .splice(0)
        .map((tempRoot) => rm(tempRoot, { force: true, recursive: true })),
    );
  });

  it("accepts the documented public package graph", async () => {
    await expect(
      inspectPublicPackageBoundaries(path.join(uiRoot, "packages")),
    ).resolves.toEqual({
      packagesRoot: path.join(uiRoot, "packages"),
      violations: [],
    });
  });

  it("rejects a prohibited dependency introduced into a reusable package", async () => {
    const tempRoot = await mkdtemp(
      path.join(os.tmpdir(), "public-package-boundaries-"),
    );
    tempRoots.push(tempRoot);
    await createPackageFamily(tempRoot, {
      "factory-emulator": {
        "@you-agent-factory/factory-visualizers": "0.0.0",
        zustand: "^5.0.0",
      },
    });
    await writeFile(
      path.join(tempRoot, "factory-emulator", "src", "index.ts"),
      "setTimeout(() => undefined, 0);\n",
    );

    const report = await inspectPublicPackageBoundaries(tempRoot);

    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "public-package-direction",
          message: expect.stringContaining(
            "@you-agent-factory/factory-emulator must not depend on @you-agent-factory/factory-visualizers",
          ),
        }),
        expect.objectContaining({
          kind: "prohibited-runtime-dependency",
          message: expect.stringContaining(
            "@you-agent-factory/factory-emulator declares unsupported runtime dependency zustand",
          ),
        }),
        expect.objectContaining({
          kind: "prohibited-runtime-ownership",
          message: expect.stringContaining("contains prohibited browser timer"),
        }),
      ]),
    );
  });
});
