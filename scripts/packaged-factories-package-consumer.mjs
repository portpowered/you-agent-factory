import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  lstat,
  mkdtemp,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { PACKAGED_FACTORIES_PACKAGE_NAME } from "./packaged-factories-package-candidate.mjs";
import {
  copyableInvocation,
  DIAGNOSTIC_PREFIX,
  isWithin,
  parseJSON,
  requireObject,
  resolvePublicArtifact,
  selectLocalizedValue,
  verifyFactory,
} from "./packaged-factories-package-consumer-contract.mjs";

const VERIFIER_DEPENDENCIES = Object.freeze([
  "ajv@8.20.0",
  "ajv-formats@3.0.1",
  "yaml@2.9.0",
]);
const REGISTRY_INSTALL_TIMEOUT_MS = 120_000;

export function npmInstallArguments(packageTarget, { registry = false } = {}) {
  return [
    "install",
    "--ignore-scripts",
    "--no-audit",
    "--no-fund",
    "--no-save",
    "--package-lock=false",
    "--workspaces=false",
    "--install-links=false",
    registry ? packageTarget : resolve(packageTarget),
    ...VERIFIER_DEPENDENCIES,
  ];
}

export function runNpmInstall(consumerDirectory, tarballPath) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn("npm", npmInstallArguments(tarballPath), {
      cwd: consumerDirectory,
      shell: process.platform === "win32",
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stderr = "";
    child.stderr.setEncoding("utf8");
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", rejectPromise);
    child.on("close", (status) => {
      if (status !== 0) {
        rejectPromise(
          new Error(
            `${DIAGNOSTIC_PREFIX} npm install failed with status ${status}\n${stderr.trim()}`,
          ),
        );
        return;
      }
      resolvePromise();
    });
  });
}

export function runNpmRegistryInstall(
  consumerDirectory,
  packageTarget,
  timeoutMs = REGISTRY_INSTALL_TIMEOUT_MS,
) {
  const arguments_ = npmInstallArguments(packageTarget, { registry: true });
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn("npm", arguments_, {
      cwd: consumerDirectory,
      shell: process.platform === "win32",
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stderr = "";
    let timedOut = false;
    const timeout = setTimeout(() => {
      timedOut = true;
      child.kill();
    }, timeoutMs);
    child.stderr.setEncoding("utf8");
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", (error) => {
      clearTimeout(timeout);
      rejectPromise(error);
    });
    child.on("close", (status) => {
      clearTimeout(timeout);
      if (timedOut) {
        rejectPromise(
          new Error(`${DIAGNOSTIC_PREFIX} npm registry install timed out`),
        );
        return;
      }
      if (status !== 0) {
        rejectPromise(
          new Error(
            `${DIAGNOSTIC_PREFIX} npm registry install failed with status ${status}\n${stderr.trim()}`,
          ),
        );
        return;
      }
      resolvePromise();
    });
  });
}

async function loadVerifierDependencies(resolveFromConsumer) {
  const [{ default: Ajv2020 }, { default: addFormats }, { parse: parseYAML }] =
    await Promise.all([
      import(pathToFileURL(resolveFromConsumer("ajv/dist/2020.js")).href),
      import(pathToFileURL(resolveFromConsumer("ajv-formats")).href),
      import(pathToFileURL(resolveFromConsumer("yaml")).href),
    ]);
  const ajv = new Ajv2020({ allErrors: true, strict: false });
  addFormats(ajv);
  return { ajv, parseYAML };
}

async function loadInstalledPackage({
  consumerRoot,
  expectedVersion,
  packageName,
}) {
  const installedPackageRoot = join(
    consumerRoot,
    "node_modules",
    ...packageName.split("/"),
  );
  if ((await lstat(installedPackageRoot)).isSymbolicLink()) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} installed package must not be a workspace link`,
    );
  }
  const packageRoot = await realpath(installedPackageRoot);
  const installedPackage = parseJSON(
    await readFile(join(packageRoot, "package.json"), "utf8"),
    "installed package manifest",
  );
  if (installedPackage.name !== packageName) {
    throw new Error(`${DIAGNOSTIC_PREFIX} installed package identity mismatch`);
  }
  if (
    typeof expectedVersion === "string" &&
    installedPackage.version !== expectedVersion
  ) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} installed version ${installedPackage.version}, want ${expectedVersion}`,
    );
  }
  return { installedPackage, packageRoot };
}

export async function verifyInstalledPackage({
  consumerDirectory,
  expectedSourceCommit,
  expectedVersion,
  packageName = PACKAGED_FACTORIES_PACKAGE_NAME,
  requestedLocale = "en-US",
  workspaceDirectory,
}) {
  const consumerRoot = resolve(consumerDirectory);
  const workspaceRoot = resolve(workspaceDirectory);
  if (isWithin(workspaceRoot, consumerRoot)) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} consumer must be outside the workspace`,
    );
  }
  const { installedPackage, packageRoot } = await loadInstalledPackage({
    consumerRoot,
    expectedVersion,
    packageName,
  });

  const resolveFromConsumer = createRequire(
    join(consumerRoot, "verify.cjs"),
  ).resolve;
  const resolveSpecifier = async (specifier) => resolveFromConsumer(specifier);
  const manifestArtifact = await resolvePublicArtifact({
    expectedPath: "generated/manifest.json",
    packageRoot,
    resolveSpecifier,
    specifier: `${packageName}/manifest`,
  });
  const schemaJSONArtifact = await resolvePublicArtifact({
    expectedPath: "schemas/factory.schema.json",
    packageRoot,
    resolveSpecifier,
    specifier: `${packageName}/schemas/factory.json`,
  });
  const schemaYAMLArtifact = await resolvePublicArtifact({
    expectedPath: "schemas/factory.schema.yaml",
    packageRoot,
    resolveSpecifier,
    specifier: `${packageName}/schemas/factory.yaml`,
  });
  const manifest = requireObject(
    parseJSON(manifestArtifact.contents, "installed contract manifest"),
    "installed contract manifest",
  );
  if (
    typeof expectedSourceCommit === "string" &&
    manifest.sourceCommit !== expectedSourceCommit
  ) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} installed provenance ${manifest.sourceCommit}, want ${expectedSourceCommit}`,
    );
  }
  if (!Array.isArray(manifest.factories) || manifest.factories.length === 0) {
    throw new Error(`${DIAGNOSTIC_PREFIX} installed manifest has no factories`);
  }
  const schemaJSON = parseJSON(
    schemaJSONArtifact.contents,
    "installed JSON Factory schema",
  );
  const { ajv, parseYAML } =
    await loadVerifierDependencies(resolveFromConsumer);
  let schemaYAML;
  try {
    schemaYAML = parseYAML(schemaYAMLArtifact.contents.toString("utf8"));
  } catch (error) {
    throw new Error(`${DIAGNOSTIC_PREFIX} installed YAML schema is invalid`, {
      cause: error,
    });
  }
  try {
    assert.deepEqual(schemaYAML, schemaJSON);
  } catch (error) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} installed JSON/YAML Factory schemas diverge\n${error.message}`,
      { cause: error },
    );
  }
  let validate;
  try {
    validate = ajv.compile(schemaJSON);
  } catch (error) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} installed Factory schema is invalid`,
      { cause: error },
    );
  }
  const factorySpecifiers = [];
  for (const entry of manifest.factories) {
    factorySpecifiers.push(
      ...(await verifyFactory({
        entry,
        packageName,
        packageRoot,
        parseYAML,
        resolveSpecifier,
        validate,
      })),
    );
  }
  const descriptionAsset = manifest.factories
    .flatMap((factory) => factory.examples ?? [])
    .find((example) => example?.description)?.description;
  const selectedDescription = selectLocalizedValue(
    descriptionAsset,
    requestedLocale,
  );
  const invocation = copyableInvocation(manifest);
  return {
    packageName,
    packageVersion: installedPackage.version,
    sourceCommit: manifest.sourceCommit,
    factoryCount: manifest.factories.length,
    verifiedSpecifiers: [
      manifestArtifact.specifier,
      schemaJSONArtifact.specifier,
      schemaYAMLArtifact.specifier,
      ...factorySpecifiers,
    ].sort((left, right) => left.localeCompare(right)),
    localizedDescription: {
      locale: requestedLocale,
      value: selectedDescription,
    },
    copyableInvocation: invocation,
  };
}

export async function installAndVerifyTarball({
  expectedSourceCommit,
  expectedVersion,
  npmInstall = runNpmInstall,
  packageName = PACKAGED_FACTORIES_PACKAGE_NAME,
  requestedLocale,
  tarballPath,
  workspaceDirectory,
}) {
  const workspaceRoot = resolve(workspaceDirectory);
  const consumerDirectory = await mkdtemp(
    join(tmpdir(), "you-packaged-factories-consumer-"),
  );
  try {
    if (isWithin(workspaceRoot, consumerDirectory)) {
      throw new Error(
        `${DIAGNOSTIC_PREFIX} temporary consumer must be outside the workspace`,
      );
    }
    await writeFile(
      join(consumerDirectory, "package.json"),
      '{"name":"packaged-factories-clean-consumer","private":true}\n',
    );
    await npmInstall(consumerDirectory, resolve(tarballPath));
    return await verifyInstalledPackage({
      consumerDirectory,
      expectedSourceCommit,
      expectedVersion,
      packageName,
      requestedLocale,
      workspaceDirectory: workspaceRoot,
    });
  } finally {
    await rm(consumerDirectory, { recursive: true, force: true });
  }
}

export async function installAndVerifyRegistryPackage({
  candidateVersion,
  consumerDirectory,
  expectedSourceCommit,
  npmInstall = runNpmRegistryInstall,
  packageName = PACKAGED_FACTORIES_PACKAGE_NAME,
  requestedLocale,
  workspaceDirectory,
}) {
  await writeFile(
    join(consumerDirectory, "package.json"),
    '{"name":"packaged-factories-registry-consumer","private":true}\n',
  );
  await npmInstall(consumerDirectory, `${packageName}@${candidateVersion}`);
  return verifyInstalledPackage({
    consumerDirectory,
    expectedSourceCommit,
    expectedVersion: candidateVersion,
    packageName,
    requestedLocale,
    workspaceDirectory,
  });
}

async function main() {
  const { values } = parseArgs({
    options: {
      "expected-source-commit": { type: "string" },
      "expected-version": { type: "string" },
      locale: { type: "string", default: "en-US" },
      tarball: { type: "string" },
      "workspace-directory": { type: "string" },
    },
    strict: true,
  });
  const evidence = await installAndVerifyTarball({
    expectedSourceCommit: values["expected-source-commit"],
    expectedVersion: values["expected-version"],
    requestedLocale: values.locale,
    tarballPath: values.tarball,
    workspaceDirectory: values["workspace-directory"],
  });
  process.stdout.write(`${JSON.stringify(evidence)}\n`);
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  main().catch((error) => {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  });
}
