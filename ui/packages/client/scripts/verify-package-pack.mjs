import { execFile } from "node:child_process";
import {
  access,
  mkdir,
  mkdtemp,
  readdir,
  readFile,
  rm,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const METADATA_FILES = ["LICENSE.md", "README.md", "package.json"];
const EXAMPLE_FILES = [
  "examples/customer-support.factory-recording.v1.json",
  "examples/customer-support.factory-visualization-layout.v1.json",
];

async function listFiles(directory, relativeRoot) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map((entry) => {
      const relativePath = path.posix.join(relativeRoot, entry.name);
      return entry.isDirectory()
        ? listFiles(path.join(directory, entry.name), relativePath)
        : relativePath;
    }),
  );
  return files.flat();
}

function collectExportTargets(exports) {
  if (typeof exports === "string") return [exports.replace(/^\.\//, "")];
  if (!exports || typeof exports !== "object") return [];
  return Object.values(exports).flatMap(collectExportTargets);
}

async function npmCommand() {
  if (process.platform !== "win32") return { args: [], executable: "npm" };
  const { stdout } = await execFileAsync("where.exe", ["npm.cmd"]);
  const command = stdout.trim().split(/\r?\n/, 1)[0];
  return {
    args: [
      path.join(
        path.dirname(command),
        "node_modules",
        "npm",
        "bin",
        "npm-cli.js",
      ),
    ],
    executable: process.execPath,
  };
}

async function runPack(destination) {
  const npm = await npmCommand();
  const { stdout } = await execFileAsync(
    npm.executable,
    [
      ...npm.args,
      "pack",
      "--json",
      "--pack-destination",
      destination,
      packageRoot,
    ],
    { cwd: packageRoot, maxBuffer: 10 * 1024 * 1024 },
  );
  const reports = JSON.parse(stdout);
  if (!Array.isArray(reports) || reports.length !== 1) {
    throw new Error("[client-package-pack] expected one npm pack report");
  }
  return reports[0];
}

function runtimeImports(source) {
  return [...source.matchAll(/(?:from\s+|import\s*\()(["'])([^"']+)\1/g)].map(
    (match) => match[2],
  );
}

function dependencyName(specifier) {
  const [first, second] = specifier.split("/");
  return first.startsWith("@") ? `${first}/${second}` : first;
}

async function verifyRuntimeBoundary(actualFiles, manifest) {
  const actualSet = new Set(actualFiles);
  const declaredDependencies = new Set(
    Object.keys(manifest.dependencies ?? {}),
  );
  for (const file of actualFiles.filter((file) => file.endsWith(".js"))) {
    const source = await readFile(path.join(packageRoot, file), "utf8");
    for (const specifier of runtimeImports(source)) {
      if (!specifier.startsWith("./") && !specifier.startsWith("../")) {
        if (!declaredDependencies.has(dependencyName(specifier))) {
          throw new Error(
            `[client-package-pack] ${file} has undeclared runtime dependency ${specifier}`,
          );
        }
        continue;
      }
      const resolved = path.posix.normalize(
        path.posix.join(path.posix.dirname(file), specifier),
      );
      if (!actualSet.has(resolved)) {
        throw new Error(
          `[client-package-pack] ${file} has missing runtime import ${resolved}`,
        );
      }
    }
  }
}

export async function packAndVerify(destination) {
  await rm(path.join(packageRoot, "dist"), { force: true, recursive: true });
  await mkdir(destination, { recursive: true });
  const report = await runPack(destination);
  const actualFiles = report.files.map(({ path: file }) => file).sort();
  const expectedFiles = [
    ...METADATA_FILES,
    ...EXAMPLE_FILES,
    ...(await listFiles(path.join(packageRoot, "dist"), "dist")),
  ].sort();
  if (JSON.stringify(actualFiles) !== JSON.stringify(expectedFiles)) {
    throw new Error(
      `[client-package-pack] unexpected tarball inventory\nactual: ${actualFiles.join(", ")}\nexpected: ${expectedFiles.join(", ")}`,
    );
  }

  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  for (const field of ["optionalDependencies", "peerDependencies"]) {
    if (manifest[field] && Object.keys(manifest[field]).length > 0) {
      throw new Error(
        `[client-package-pack] published package declares ${field}`,
      );
    }
  }
  const dependencies = Object.keys(manifest.dependencies ?? {}).sort();
  if (
    JSON.stringify(dependencies) !==
    JSON.stringify(["ajv", "ajv-formats", "marked"])
  ) {
    throw new Error(
      `[client-package-pack] unexpected runtime dependencies: ${dependencies.join(", ")}`,
    );
  }
  for (const target of collectExportTargets(manifest.exports)) {
    if (!actualFiles.includes(target)) {
      throw new Error(`[client-package-pack] missing export target ${target}`);
    }
  }
  await verifyRuntimeBoundary(actualFiles, manifest);

  const tarballPath = path.join(destination, report.filename);
  await access(tarballPath);
  return { files: actualFiles, tarballPath };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const temporaryDirectory = await mkdtemp(
    path.join(tmpdir(), "you-client-pack-"),
  );
  try {
    const result = await packAndVerify(temporaryDirectory);
    process.stdout.write(
      `[client-package-pack] verified ${result.files.length} files\n`,
    );
  } finally {
    await rm(temporaryDirectory, { force: true, recursive: true });
  }
}
