// @vitest-environment node

import {
  mkdir,
  mkdtemp,
  realpath,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";

import { assertScopedComponentsResolved } from "./verify-fresh-npm-install.mjs";

async function createResolvedInstallFixture() {
  const installRoot = await mkdtemp(
    path.join(os.tmpdir(), "verify-fresh-npm-install-fixture-"),
  );
  const componentsDir = path.join(installRoot, "packages", "components");
  const scopedDir = path.join(
    installRoot,
    "node_modules",
    "@you-agent-factory",
  );

  await mkdir(scopedDir, { recursive: true });
  await mkdir(componentsDir, { recursive: true });
  await writeFile(
    path.join(componentsDir, "package.json"),
    JSON.stringify({ name: "@you-agent-factory/components" }, null, 2),
  );
  await symlink(
    path.join(installRoot, "packages", "components"),
    path.join(scopedDir, "components"),
    process.platform === "win32" ? "junction" : "dir",
  );

  return installRoot;
}

describe("assertScopedComponentsResolved", () => {
  it("accepts a scoped local file dependency resolved to packages/components", async () => {
    const installRoot = await createResolvedInstallFixture();

    try {
      const expectedResolvedPath = await realpath(
        path.join(installRoot, "packages", "components"),
      );

      await expect(
        assertScopedComponentsResolved(installRoot),
      ).resolves.toEqual({
        packageName: "@you-agent-factory/components",
        resolvedPath: expectedResolvedPath,
      });
    } finally {
      await rm(installRoot, { force: true, recursive: true });
    }
  });

  it("rejects installs that point at a different directory", async () => {
    const installRoot = await createResolvedInstallFixture();
    const wrongTarget = path.join(installRoot, "packages", "wrong-components");

    try {
      await mkdir(wrongTarget, { recursive: true });
      await rm(
        path.join(
          installRoot,
          "node_modules",
          "@you-agent-factory",
          "components",
        ),
      );
      await symlink(
        wrongTarget,
        path.join(
          installRoot,
          "node_modules",
          "@you-agent-factory",
          "components",
        ),
        process.platform === "win32" ? "junction" : "dir",
      );

      await expect(assertScopedComponentsResolved(installRoot)).rejects.toThrow(
        /Expected @you-agent-factory\/components to resolve from packages[\\/]components/,
      );
    } finally {
      await rm(installRoot, { force: true, recursive: true });
    }
  });
});
