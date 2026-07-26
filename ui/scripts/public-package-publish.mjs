import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync } from "node:fs";
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

import { assertPackedExportTargets } from "../../scripts/package-export-validation.mjs";
import {
  assertCandidateSetEvidence,
  FRONTEND_ONLY_CANDIDATE_SCOPE,
} from "../../scripts/public-package-set.mjs";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const semverPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export const PUBLIC_PACKAGES = Object.freeze([
  { name: "@you-agent-factory/client", directory: "client" },
  { name: "@you-agent-factory/factory-replay", directory: "factory-replay" },
  {
    name: "@you-agent-factory/factory-emulator",
    directory: "factory-emulator",
  },
  { name: "@you-agent-factory/components", directory: "components" },
  {
    name: "@you-agent-factory/factory-visualizers",
    directory: "factory-visualizers",
  },
]);

const publicPackageNames = new Set(PUBLIC_PACKAGES.map(({ name }) => name));

export function assertPublishVersion(version) {
  if (typeof version !== "string" || !semverPattern.test(version)) {
    throw new Error(`Invalid public package version: ${version ?? "missing"}`);
  }
  return version;
}

export function patchPublicPackageManifest(manifest, version) {
  const next = structuredClone(manifest);
  next.version = assertPublishVersion(version);
  for (const field of ["dependencies", "devDependencies", "peerDependencies"]) {
    if (!next[field]) continue;
    for (const dependencyName of Object.keys(next[field])) {
      if (publicPackageNames.has(dependencyName)) {
        next[field][dependencyName] = version;
      }
    }
  }
  return next;
}

export function assertFrontendCandidateEvidence(evidence) {
  assertPublishVersion(evidence?.version);
  assertCandidateSetEvidence(evidence, FRONTEND_ONLY_CANDIDATE_SCOPE);
  return evidence;
}

const npmPackArguments = (stagedDirectory, outputDirectory) => [
  "pack",
  stagedDirectory,
  "--json",
  "--ignore-scripts",
  "--pack-destination",
  outputDirectory,
];

function run(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: options.cwd,
      env: process.env,
      shell: false,
      stdio: options.capture ? ["ignore", "pipe", "pipe"] : "inherit",
    });
    let stdout = "";
    let stderr = "";
    child.stdout?.on("data", (chunk) => (stdout += chunk));
    child.stderr?.on("data", (chunk) => (stderr += chunk));
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      if (code === 0) return resolve({ stdout, stderr });
      const outcome = code === null ? `signal ${signal}` : `exit code ${code}`;
      reject(
        Object.assign(
          new Error(
            `${command} ${args.join(" ")} failed with ${outcome}\n${stderr}`,
          ),
          {
            code,
            stdout,
            stderr,
          },
        ),
      );
    });
  });
}

function runNpm(args, options = {}) {
  if (process.platform !== "win32") return run("npm", args, options);
  const configuredNpmCli = process.env.npm_execpath;
  const npmCli =
    configuredNpmCli &&
    /(?:^|[\\/])npm(?:-cli)?\.[cm]?js$/i.test(configuredNpmCli) &&
    existsSync(configuredNpmCli)
      ? configuredNpmCli
      : path.join(
          path.dirname(process.execPath),
          "node_modules/npm/bin/npm-cli.js",
        );
  if (!existsSync(npmCli)) {
    throw new Error("npm CLI could not be resolved safely on Windows");
  }
  return run(process.execPath, [npmCli, ...args], options);
}

export function packCandidate({ stagedDirectory, outputDirectory }) {
  return runNpm(npmPackArguments(stagedDirectory, outputDirectory), {
    cwd: uiRoot,
    capture: true,
  });
}

async function stagePackage({ packageSpec, version, stagingRoot }) {
  const sourceDirectory = path.join(uiRoot, "packages", packageSpec.directory);
  const stagedDirectory = path.join(stagingRoot, packageSpec.directory);
  await run("bun", ["run", "build"], { cwd: sourceDirectory });
  await cp(sourceDirectory, stagedDirectory, {
    recursive: true,
    filter: (entry) => !entry.split(path.sep).includes("node_modules"),
  });
  const manifestPath = path.join(stagedDirectory, "package.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const patched = patchPublicPackageManifest(manifest, version);
  if (patched.name !== packageSpec.name) {
    throw new Error(
      `Expected ${packageSpec.name} at ${sourceDirectory}, found ${patched.name}`,
    );
  }
  await writeFile(manifestPath, `${JSON.stringify(patched, null, 2)}\n`);
  return { manifest: patched, stagedDirectory };
}

async function snapshotBuildOutputs(stagingRoot) {
  const snapshots = [];
  for (const packageSpec of PUBLIC_PACKAGES) {
    const sourceDirectory = path.join(
      uiRoot,
      "packages",
      packageSpec.directory,
    );
    const outputDirectory = path.join(sourceDirectory, "dist");
    const backupDirectory = path.join(
      stagingRoot,
      ".build-output-backups",
      packageSpec.directory,
    );
    try {
      await stat(outputDirectory);
      await cp(outputDirectory, backupDirectory, { recursive: true });
      snapshots.push({ outputDirectory, backupDirectory });
    } catch (error) {
      if (error?.code === "ENOENT") snapshots.push({ outputDirectory });
      else throw error;
    }
  }
  return snapshots;
}

async function restoreBuildOutputs(snapshots) {
  for (const { outputDirectory, backupDirectory } of snapshots) {
    await rm(outputDirectory, { recursive: true, force: true });
    if (backupDirectory)
      await cp(backupDirectory, outputDirectory, { recursive: true });
  }
}

export async function preparePublicPackageCandidates({
  version,
  outputDirectory,
}) {
  assertPublishVersion(version);
  const resolvedOutput = path.resolve(outputDirectory);
  const stagingRoot = await mkdtemp(path.join(uiRoot, ".you-public-packages-"));
  await mkdir(resolvedOutput, { recursive: true });
  await run("bun", ["run", "link:public-package-dependencies"], {
    cwd: uiRoot,
  });
  const buildOutputs = await snapshotBuildOutputs(stagingRoot);
  const candidates = [];
  try {
    for (const packageSpec of PUBLIC_PACKAGES) {
      const { manifest, stagedDirectory } = await stagePackage({
        packageSpec,
        version,
        stagingRoot,
      });
      const { stdout } = await packCandidate({
        stagedDirectory,
        outputDirectory: resolvedOutput,
      });
      const [report] = JSON.parse(stdout);
      if (report?.name !== packageSpec.name || report?.version !== version) {
        throw new Error(
          `npm pack returned unexpected identity for ${packageSpec.name}`,
        );
      }
      assertPackedExportTargets(report.name, manifest.exports, report.files);
      candidates.push({
        name: report.name,
        version: report.version,
        filename: report.filename,
        integrity: report.integrity,
        shasum: report.shasum,
      });
    }
    const evidence = {
      scope: FRONTEND_ONLY_CANDIDATE_SCOPE,
      version,
      packages: candidates,
    };
    await writeFile(
      path.join(resolvedOutput, "public-package-candidates.json"),
      `${JSON.stringify(evidence, null, 2)}\n`,
    );
    return evidence;
  } finally {
    await restoreBuildOutputs(buildOutputs);
    await rm(stagingRoot, { recursive: true, force: true });
  }
}

async function registryShasum(packageName, version) {
  try {
    const { stdout } = await runNpm(
      ["view", `${packageName}@${version}`, "dist.shasum", "--json"],
      {
        cwd: uiRoot,
        capture: true,
      },
    );
    const shasum = JSON.parse(stdout);
    if (typeof shasum !== "string" || !/^[a-f0-9]{40}$/.test(shasum)) {
      throw new Error(
        `Registry returned an invalid digest for ${packageName}@${version}`,
      );
    }
    return shasum;
  } catch (error) {
    if (error?.code === 1 && /E404|404 Not Found/.test(error.stderr ?? ""))
      return null;
    throw error;
  }
}

async function verifyRegistryVersion(packageName, version, expectedShasum) {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const shasum = await registryShasum(packageName, version);
    if (shasum === expectedShasum) return;
    if (shasum !== null) {
      throw new Error(`Registry digest conflict for ${packageName}@${version}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 5_000));
  }
  throw new Error(
    `Published version did not become visible: ${packageName}@${version}`,
  );
}

export async function publishPublicPackageCandidates({
  candidateDirectory,
  tag,
  provenance,
}) {
  if (!/^[a-z][a-z0-9-]*$/.test(tag ?? ""))
    throw new Error(`Invalid npm dist-tag: ${tag}`);
  const resolvedDirectory = path.resolve(candidateDirectory);
  const evidence = JSON.parse(
    await readFile(
      path.join(resolvedDirectory, "public-package-candidates.json"),
      "utf8",
    ),
  );
  assertFrontendCandidateEvidence(evidence);
  for (const packageSpec of PUBLIC_PACKAGES) {
    const candidate = evidence.packages.find(
      ({ name }) => name === packageSpec.name,
    );
    if (!candidate || candidate.version !== evidence.version) {
      throw new Error(`Missing exact candidate for ${packageSpec.name}`);
    }
    if (path.basename(candidate.filename) !== candidate.filename) {
      throw new Error(`Invalid candidate filename for ${packageSpec.name}`);
    }
    const tarballPath = path.join(resolvedDirectory, candidate.filename);
    const shasum = createHash("sha1")
      .update(await readFile(tarballPath))
      .digest("hex");
    if (shasum !== candidate.shasum) {
      throw new Error(`Candidate digest mismatch for ${packageSpec.name}`);
    }
    const registryDigest = await registryShasum(
      candidate.name,
      candidate.version,
    );
    if (registryDigest !== null && registryDigest !== candidate.shasum) {
      throw new Error(`Registry digest conflict for ${packageSpec.name}`);
    }
    if (registryDigest === null) {
      const args = ["publish", tarballPath, "--tag", tag, "--access", "public"];
      if (provenance) args.push("--provenance");
      await runNpm(args, { cwd: uiRoot });
    }
    await verifyRegistryVersion(
      candidate.name,
      candidate.version,
      candidate.shasum,
    );
  }
  return evidence;
}

async function main() {
  const { values } = parseArgs({
    options: {
      action: { type: "string" },
      version: { type: "string" },
      "output-directory": { type: "string" },
      "candidate-directory": { type: "string" },
      tag: { type: "string" },
      provenance: { type: "boolean", default: false },
    },
    strict: true,
  });
  const result =
    values.action === "prepare"
      ? await preparePublicPackageCandidates({
          version: values.version,
          outputDirectory: values["output-directory"],
        })
      : values.action === "publish"
        ? await publishPublicPackageCandidates({
            candidateDirectory: values["candidate-directory"],
            tag: values.tag,
            provenance: values.provenance,
          })
        : (() => {
            throw new Error(
              `Unsupported public package action: ${values.action ?? "missing"}`,
            );
          })();
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

const isMain =
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMain)
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
