import { execFile } from "node:child_process";
import {
  cp,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rename,
  rm,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageName = "@you-agent-factory/packaged-factories";
const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(scriptDirectory, "..");
const packageScope = path.join(uiRoot, "node_modules", "@you-agent-factory");
const destination = path.join(packageScope, "packaged-factories");

async function readPackageDefinition(directory) {
  return JSON.parse(
    await readFile(path.join(directory, "package.json"), "utf8"),
  );
}

async function resolvePackageSource() {
  if (process.env.AGENT_FACTORY_PACKAGED_FACTORIES_SOURCE) {
    return path.resolve(process.env.AGENT_FACTORY_PACKAGED_FACTORIES_SOURCE);
  }

  const uiDefinition = await readPackageDefinition(uiRoot);
  const declaredDependency = uiDefinition.dependencies?.[packageName];
  if (
    typeof declaredDependency !== "string" ||
    !declaredDependency.startsWith("file:")
  ) {
    throw new Error(
      `[packaged-factories-install] ${packageName} must be a declared file dependency`,
    );
  }
  return path.resolve(uiRoot, declaredDependency.slice("file:".length));
}

async function assertExpectedPackage(directory, description) {
  const definition = await readPackageDefinition(directory);
  if (definition.name !== packageName) {
    throw new Error(
      `[packaged-factories-install] ${description} has package name ${definition.name ?? "<missing>"}, expected ${packageName}`,
    );
  }
}

async function pathExists(targetPath) {
  try {
    await lstat(targetPath);
    return true;
  } catch (error) {
    if (error?.code === "ENOENT") {
      return false;
    }
    throw error;
  }
}

function runNpm(arguments_) {
  const command =
    process.platform === "win32" ? (process.env.ComSpec ?? "cmd.exe") : "npm";
  const commandArguments =
    process.platform === "win32"
      ? ["/d", "/s", "/c", "npm.cmd", ...arguments_]
      : arguments_;
  return execFileAsync(command, commandArguments, {
    maxBuffer: 10 * 1024 * 1024,
  });
}

async function replaceDeclaredDependency(installedCandidate) {
  await mkdir(packageScope, { recursive: true });
  await assertExpectedPackage(installedCandidate, "packed candidate");

  if (await pathExists(destination)) {
    await assertExpectedPackage(destination, "declared dependency");
    await rm(destination, { force: true, recursive: true });
  }

  const stagedDestination = path.join(
    packageScope,
    `.packaged-factories-candidate-${process.pid}`,
  );
  await rm(stagedDestination, { force: true, recursive: true });
  try {
    await cp(installedCandidate, stagedDestination, {
      dereference: true,
      recursive: true,
    });
    await rename(stagedDestination, destination);
  } finally {
    await rm(stagedDestination, { force: true, recursive: true });
  }

  const [resolvedDestination, resolvedScope] = await Promise.all([
    realpath(destination),
    realpath(packageScope),
  ]);
  const relativeDestination = path.relative(resolvedScope, resolvedDestination);
  if (
    relativeDestination.startsWith("..") ||
    path.isAbsolute(relativeDestination)
  ) {
    throw new Error(
      "[packaged-factories-install] installed candidate escaped the UI package scope",
    );
  }
}

async function installCandidate() {
  const packageSource = await resolvePackageSource();
  await assertExpectedPackage(packageSource, "package source");
  const temporaryRoot = await mkdtemp(
    path.join(os.tmpdir(), "you-packaged-factories-ui-"),
  );

  try {
    const packDirectory = path.join(temporaryRoot, "pack");
    const consumerDirectory = path.join(temporaryRoot, "consumer");
    await Promise.all([
      mkdir(packDirectory, { recursive: true }),
      mkdir(consumerDirectory, { recursive: true }),
    ]);

    const { stdout } = await runNpm([
      "pack",
      "--json",
      "--pack-destination",
      packDirectory,
      packageSource,
    ]);
    const reports = JSON.parse(stdout);
    const filename = reports?.[0]?.filename;
    if (reports.length !== 1 || typeof filename !== "string") {
      throw new Error(
        "[packaged-factories-install] npm pack returned no unique candidate",
      );
    }

    const tarballPath = path.join(packDirectory, filename);
    await runNpm([
      "install",
      "--ignore-scripts",
      "--no-audit",
      "--no-fund",
      "--no-save",
      "--package-lock=false",
      "--workspaces=false",
      "--install-links=false",
      "--prefix",
      consumerDirectory,
      tarballPath,
    ]);

    await replaceDeclaredDependency(
      path.join(
        consumerDirectory,
        "node_modules",
        "@you-agent-factory",
        "packaged-factories",
      ),
    );
  } finally {
    await rm(temporaryRoot, { force: true, recursive: true });
  }
}

await installCandidate();
console.log(
  "[packaged-factories-install] installed physical package candidate from declared dependency",
);
