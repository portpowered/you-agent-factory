import { execFile } from "node:child_process";
import {
  access,
  cp,
  lstat,
  mkdtemp,
  readFile,
  realpath,
  rm,
  symlink,
} from "node:fs/promises";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageName = "@you-agent-factory/packaged-factories";
const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(scriptDirectory, "..");
const repositoryPackages = path.resolve(uiRoot, "..", "packages");
const installedPackage = path.join(
  uiRoot,
  "node_modules",
  "@you-agent-factory",
  "packaged-factories",
);

function isWithin(candidate, parent) {
  const relative = path.relative(parent, candidate);
  return (
    relative === "" ||
    (!relative.startsWith("..") && !path.isAbsolute(relative))
  );
}

async function assertPhysicalInstalledPackage() {
  const packageStat = await lstat(installedPackage);
  if (packageStat.isSymbolicLink()) {
    throw new Error(
      "[packaged-factories-consumer] dependency must be a physical installed candidate",
    );
  }

  const [resolvedPackage, resolvedNodeModules] = await Promise.all([
    realpath(installedPackage),
    realpath(path.join(uiRoot, "node_modules")),
  ]);
  if (!isWithin(resolvedPackage, resolvedNodeModules)) {
    throw new Error(
      "[packaged-factories-consumer] dependency resolves outside UI node_modules",
    );
  }

  const definition = JSON.parse(
    await readFile(path.join(resolvedPackage, "package.json"), "utf8"),
  );
  if (definition.name !== packageName) {
    throw new Error(
      `[packaged-factories-consumer] installed package name is ${definition.name ?? "<missing>"}`,
    );
  }
}

async function assertPublicExportsResolve() {
  const require = createRequire(path.join(uiRoot, "package.json"));
  const manifestPath = require.resolve(`${packageName}/manifest`);
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const specifiers = [
    `${packageName}/manifest`,
    `${packageName}/schemas/factory.json`,
    ...manifest.factories.flatMap(({ slug }) => [
      `${packageName}/factories/${slug}.json`,
      `${packageName}/factories/${slug}.yaml`,
    ]),
  ];

  await Promise.all(
    specifiers.map((specifier) => access(require.resolve(specifier))),
  );
  return specifiers.length;
}

async function runRelocatedBuild() {
  const temporaryRoot = await mkdtemp(
    path.join(os.tmpdir(), "you-packaged-factories-relocated-ui-"),
  );
  const relocatedUi = path.join(temporaryRoot, "ui");

  try {
    await cp(uiRoot, relocatedUi, {
      filter(source) {
        const relative = path.relative(uiRoot, source);
        const firstSegment = relative.split(path.sep)[0];
        return ![
          "coverage",
          "dist",
          "node_modules",
          "storybook-static",
        ].includes(firstSegment);
      },
      recursive: true,
    });
    await cp(repositoryPackages, path.join(temporaryRoot, "packages"), {
      filter(source) {
        const relative = path.relative(repositoryPackages, source);
        return relative.split(path.sep)[0] !== "packaged-factories";
      },
      recursive: true,
    });
    await symlink(
      path.join(uiRoot, "node_modules"),
      path.join(relocatedUi, "node_modules"),
      process.platform === "win32" ? "junction" : "dir",
    );

    const unavailableSource = path.resolve(
      relocatedUi,
      "..",
      "packages",
      "packaged-factories",
    );
    await access(unavailableSource).then(
      () => {
        throw new Error(
          "[packaged-factories-consumer] relocated build unexpectedly retained repository package sources",
        );
      },
      (error) => {
        if (error?.code !== "ENOENT") {
          throw error;
        }
      },
    );

    await execFileAsync(
      "node",
      ["scripts/generate-packaged-factory-resolver.mjs", "--check"],
      {
        cwd: relocatedUi,
        maxBuffer: 10 * 1024 * 1024,
      },
    );
    await execFileAsync("bun", ["x", "tsc", "-b"], {
      cwd: relocatedUi,
      maxBuffer: 10 * 1024 * 1024,
    });
    await execFileAsync("bun", ["x", "vite", "build"], {
      cwd: relocatedUi,
      maxBuffer: 10 * 1024 * 1024,
    });
    await execFileAsync("node", ["scripts/normalize-dist-output.mjs"], {
      cwd: relocatedUi,
      maxBuffer: 10 * 1024 * 1024,
    });
  } finally {
    await rm(temporaryRoot, { force: true, recursive: true });
  }
}

await assertPhysicalInstalledPackage();
const exportCount = await assertPublicExportsResolve();
await runRelocatedBuild();
console.log(
  `[packaged-factories-consumer] relocated UI built from ${exportCount} installed public exports`,
);
