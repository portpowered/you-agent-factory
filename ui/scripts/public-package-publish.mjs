import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const semverPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export const PUBLIC_PACKAGES = Object.freeze([
  { name: "@you-agent-factory/client", directory: "client", build: true },
  {
    name: "@you-agent-factory/factory-replay",
    directory: "factory-replay",
    build: true,
  },
  {
    name: "@you-agent-factory/factory-emulator",
    directory: "factory-emulator",
    build: true,
  },
  {
    name: "@you-agent-factory/components",
    directory: "components",
    build: true,
  },
  {
    name: "@you-agent-factory/factory-visualizers",
    directory: "factory-visualizers",
    build: true,
  },
]);

export const RELEASE_PUBLIC_PACKAGES = Object.freeze([
  {
    name: "@you-agent-factory/api",
    directory: "api",
    sourceDirectory: "../packages/api",
    build: false,
  },
  {
    name: "@you-agent-factory/packaged-factories",
    directory: "packaged-factories",
    sourceDirectory: "../packages/packaged-factories",
    build: false,
  },
  ...PUBLIC_PACKAGES,
]);

const publicPackageNames = new Set(
  RELEASE_PUBLIC_PACKAGES.map(({ name }) => name),
);

function collectExportTargets(exports) {
  if (typeof exports === "string") return [exports.replace(/^\.\//, "")];
  if (!exports || typeof exports !== "object") return [];
  return Object.values(exports).flatMap(collectExportTargets);
}

function matchesExportTarget(file, target) {
  if (!target.includes("*")) return file === target;
  const [prefix, suffix] = target.split("*", 2);
  return file.startsWith(prefix) && file.endsWith(suffix);
}

export function assertPackedExportTargets(packageName, exports, files) {
  const packedFiles = new Set(
    (files ?? []).map(({ path: file }) => file?.replaceAll("\\", "/")),
  );
  for (const target of collectExportTargets(exports)) {
    if (![...packedFiles].some((file) => matchesExportTarget(file, target))) {
      throw new Error(`${packageName} candidate omits export target ${target}`);
    }
  }
}

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
  if (!process.env.npm_execpath) {
    throw new Error("npm_execpath is required to run npm safely on Windows");
  }
  return run(process.execPath, [process.env.npm_execpath, ...args], options);
}

async function stagePackage({ packageSpec, version, stagingRoot }) {
  const sourceDirectory = path.resolve(
    uiRoot,
    packageSpec.sourceDirectory ?? `packages/${packageSpec.directory}`,
  );
  const stagedDirectory = path.join(stagingRoot, packageSpec.directory);
  if (packageSpec.build) {
    await run("bun", ["run", "build"], { cwd: sourceDirectory });
  }
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

export async function preparePublicPackageCandidates({
  version,
  outputDirectory,
  includeReleasePackages = false,
}) {
  assertPublishVersion(version);
  const resolvedOutput = path.resolve(outputDirectory);
  const stagingRoot = await mkdtemp(
    path.join(tmpdir(), "you-public-packages-"),
  );
  await mkdir(resolvedOutput, { recursive: true });
  await run("bun", ["run", "link:public-package-dependencies"], {
    cwd: uiRoot,
  });
  const packageSpecs = includeReleasePackages
    ? RELEASE_PUBLIC_PACKAGES
    : PUBLIC_PACKAGES;
  const candidates = [];
  try {
    for (const packageSpec of packageSpecs) {
      const { manifest, stagedDirectory } = await stagePackage({
        packageSpec,
        version,
        stagingRoot,
      });
      const { stdout } = await runNpm(
        [
          "pack",
          stagedDirectory,
          "--json",
          "--ignore-scripts",
          "--pack-destination",
          resolvedOutput,
        ],
        { cwd: uiRoot, capture: true },
      );
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
      version,
      scope: includeReleasePackages ? "release" : "ui",
      packages: candidates,
    };
    await writeFile(
      path.join(resolvedOutput, "public-package-candidates.json"),
      `${JSON.stringify(evidence, null, 2)}\n`,
    );
    return evidence;
  } finally {
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
  assertPublishVersion(evidence.version);
  const packageSpecs =
    evidence.scope === "release"
      ? RELEASE_PUBLIC_PACKAGES
      : evidence.scope === "ui"
        ? PUBLIC_PACKAGES
        : null;
  if (!packageSpecs || evidence.packages.length !== packageSpecs.length) {
    throw new Error("Public package candidate set is incomplete");
  }
  if (
    new Set(evidence.packages.map(({ name }) => name)).size !==
    evidence.packages.length
  ) {
    throw new Error(
      "Public package candidate set contains duplicate package names",
    );
  }
  for (const packageSpec of packageSpecs) {
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
      "include-release-packages": { type: "boolean", default: false },
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
          includeReleasePackages: values["include-release-packages"],
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
