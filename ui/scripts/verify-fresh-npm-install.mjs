import { execFile } from "node:child_process";
import {
  access,
  cp,
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultUiDir = path.resolve(scriptDir, "..");
const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components";
const COMPONENTS_RELATIVE_PATH = path.join("packages", "components");

async function createIsolatedWorkDir(prefix) {
  const bases = [
    os.tmpdir(),
    "/tmp",
    path.join(defaultUiDir, "node_modules", ".cache"),
  ];
  const seen = new Set();

  for (const base of bases) {
    const resolvedBase = path.resolve(base);
    if (seen.has(resolvedBase)) {
      continue;
    }
    seen.add(resolvedBase);

    try {
      await mkdir(resolvedBase, { recursive: true });
      return await mkdtemp(path.join(resolvedBase, prefix));
    } catch (error) {
      if (error?.code === "ENOENT") {
        continue;
      }
      throw error;
    }
  }

  throw new Error(
    "Could not create a temporary directory for fresh npm install verification.",
  );
}

/**
 * Assert the dashboard npm install graph resolved the scoped local components package.
 *
 * @param {string} installRoot
 * @returns {Promise<{ packageName: string; resolvedPath: string }>}
 */
export async function assertScopedComponentsResolved(installRoot) {
  const installedPackagePath = path.join(
    installRoot,
    "node_modules",
    "@you-agent-factory",
    "components",
  );
  const expectedComponentsPath = path.join(
    installRoot,
    COMPONENTS_RELATIVE_PATH,
  );

  await access(installedPackagePath);

  const [resolvedInstallPath, resolvedComponentsPath] = await Promise.all([
    realpath(installedPackagePath),
    realpath(expectedComponentsPath),
  ]);

  if (resolvedInstallPath !== resolvedComponentsPath) {
    throw new Error(
      [
        `Expected ${COMPONENTS_PACKAGE_NAME} to resolve from ${COMPONENTS_RELATIVE_PATH}.`,
        `Installed path: ${resolvedInstallPath}`,
        `Expected path: ${resolvedComponentsPath}`,
      ].join("\n"),
    );
  }

  const manifest = JSON.parse(
    await readFile(path.join(resolvedComponentsPath, "package.json"), "utf8"),
  );

  if (manifest.name !== COMPONENTS_PACKAGE_NAME) {
    throw new Error(
      `Expected ${COMPONENTS_RELATIVE_PATH}/package.json name ${COMPONENTS_PACKAGE_NAME}, received ${manifest.name ?? "<missing>"}.`,
    );
  }

  return {
    packageName: manifest.name,
    resolvedPath: resolvedInstallPath,
  };
}

/**
 * Run a fresh npm install for the dashboard dependency graph in an isolated workspace.
 *
 * @param {{ uiDir?: string; workDir?: string; npmCommand?: string }} [options]
 */
export async function verifyFreshNpmInstall(options = {}) {
  const uiDir = path.resolve(options.uiDir ?? defaultUiDir);
  const workDir =
    options.workDir ??
    (await createIsolatedWorkDir("verify-fresh-npm-install-"));
  const npmCommand = options.npmCommand ?? "npm";
  const ownsWorkDir = !options.workDir;

  try {
    await cp(
      path.join(uiDir, "package.json"),
      path.join(workDir, "package.json"),
    );
    await cp(path.join(uiDir, "packages"), path.join(workDir, "packages"), {
      recursive: true,
    });

    const packageLockPath = path.join(uiDir, "package-lock.json");
    try {
      await access(packageLockPath);
      await cp(packageLockPath, path.join(workDir, "package-lock.json"));
    } catch {
      // Fresh installs without a lockfile still prove manifest resolution.
    }

    await execFileAsync(npmCommand, ["install"], {
      cwd: workDir,
      env: process.env,
      maxBuffer: 10 * 1024 * 1024,
    });

    return await assertScopedComponentsResolved(workDir);
  } finally {
    if (ownsWorkDir) {
      await rm(workDir, { force: true, recursive: true });
    }
  }
}

async function main() {
  const uiDir = process.env.AGENT_FACTORY_UI_DIR
    ? path.resolve(process.env.AGENT_FACTORY_UI_DIR)
    : defaultUiDir;

  const result = await verifyFreshNpmInstall({ uiDir });

  console.log(
    [
      "Fresh npm install verification passed.",
      `${COMPONENTS_PACKAGE_NAME} resolved from ${COMPONENTS_RELATIVE_PATH}.`,
      `Resolved path: ${result.resolvedPath}`,
    ].join("\n"),
  );
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main().catch((error) => {
    console.error(
      error instanceof Error
        ? error.message
        : "Fresh npm install verification failed.",
    );
    process.exitCode = 1;
  });
}
