import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { expect, test } from "vitest";

import { inspectPackagedFactoriesBoundary } from "./check-packaged-factories-boundary.mjs";

const execFileAsync = promisify(execFile);
const scriptPath = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "check-packaged-factories-boundary.mjs",
);

async function createUiFixture({ manifest, sources }) {
  const tempRoot = await mkdtemp(
    path.join(os.tmpdir(), "packaged-factories-boundary-"),
  );
  const uiRoot = path.join(tempRoot, "ui");
  const sourceRoot = path.join(uiRoot, "src");
  await mkdir(sourceRoot, { recursive: true });
  await writeFile(
    path.join(uiRoot, "package.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );

  for (const [relativeFilePath, contents] of Object.entries(sources)) {
    const filePath = path.join(sourceRoot, relativeFilePath);
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, contents);
  }

  return { sourceRoot, tempRoot, uiRoot };
}

function fixtureEnvironment(fixture) {
  return {
    ...process.env,
    AGENT_FACTORY_PACKAGED_FACTORIES_SOURCE_ROOT: fixture.sourceRoot,
    AGENT_FACTORY_PACKAGED_FACTORIES_UI_ROOT: fixture.uiRoot,
  };
}

test("the guard rejects manifest and source violations through its CLI", async () => {
  const fixture = await createUiFixture({
    manifest: {
      name: "synthetic-dashboard",
      peerDependencies: {
        "@you-agent-factory/packaged-factories": "*",
      },
      bundledDependencies: ["@you-agent-factory/packaged-factories"],
    },
    sources: {
      "catalog.ts": `
        import "@you-agent-factory/packaged-factories";

        export async function loadCatalog() {
          return import("@you-agent-factory/packaged-factories/manifest");
        }
      `,
    },
  });

  try {
    const report = await inspectPackagedFactoriesBoundary(fixture);
    expect(report.violations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: "manifest-dependency",
          section: "peerDependencies",
        }),
        expect.objectContaining({
          kind: "manifest-dependency",
          section: "bundledDependencies",
        }),
        expect.objectContaining({
          kind: "source-import",
          specifier: "@you-agent-factory/packaged-factories",
          syntax: "static import",
        }),
        expect.objectContaining({
          kind: "source-import",
          specifier: "@you-agent-factory/packaged-factories/manifest",
          syntax: "dynamic import",
        }),
      ]),
    );

    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        env: fixtureEnvironment(fixture),
      }),
    ).rejects.toMatchObject({
      code: 1,
      stderr: expect.stringContaining(
        "package.json [peerDependencies] declares prohibited package",
      ),
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        env: fixtureEnvironment(fixture),
      }),
    ).rejects.toMatchObject({
      stderr: expect.stringContaining(
        "src/catalog.ts:2:9 uses static import to import prohibited package",
      ),
    });
  } finally {
    await rm(fixture.tempRoot, { force: true, recursive: true });
  }
});

test("the guard passes a compliant isolated fixture", async () => {
  const fixture = await createUiFixture({
    manifest: {
      name: "synthetic-dashboard",
      dependencies: { react: "*" },
      devDependencies: { typescript: "*" },
      optionalDependencies: { "optional-native-addon": "*" },
      peerDependencies: { react: "*" },
      bundledDependencies: ["react"],
    },
    sources: {
      "catalog.ts": `
        import { getCatalog } from "./catalog-client";

        export async function loadCatalog() {
          return import("./catalog-client").then(({ getCatalog: load }) =>
            load(),
          );
        }

        void getCatalog;
      `,
      "catalog-client.ts": "export function getCatalog() { return []; }\n",
    },
  });

  try {
    await expect(
      inspectPackagedFactoriesBoundary(fixture),
    ).resolves.toMatchObject({
      violations: [],
    });
    await expect(
      execFileAsync(process.execPath, [scriptPath], {
        env: fixtureEnvironment(fixture),
      }),
    ).resolves.toMatchObject({
      stdout: "Packaged Factories dashboard boundary passed.\n",
      stderr: "",
    });
  } finally {
    await rm(fixture.tempRoot, { force: true, recursive: true });
  }
});
