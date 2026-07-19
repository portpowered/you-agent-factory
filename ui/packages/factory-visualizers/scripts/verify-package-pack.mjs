import { execFile } from "node:child_process";
import { access, mkdtemp, readdir, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

import { verifyPackageBoundary } from "./check-package-boundary.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const metadataFiles = ["LICENSE.md", "README.md", "package.json"];

async function listFiles(directory, relativeRoot) {
  const entries = await readdir(directory, { withFileTypes: true });
  return (
    await Promise.all(
      entries.map((entry) => {
        const relativePath = path.posix.join(relativeRoot, entry.name);
        return entry.isDirectory()
          ? listFiles(path.join(directory, entry.name), relativePath)
          : relativePath;
      }),
    )
  ).flat();
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

export async function packAndVerify(destination) {
  await execFileAsync(
    process.execPath,
    [path.join(packageRoot, "scripts", "build-package.mjs")],
    {
      cwd: packageRoot,
      maxBuffer: 20 * 1024 * 1024,
    },
  );
  await verifyPackageBoundary();
  const npm = await npmCommand();
  const { stdout } = await execFileAsync(
    npm.executable,
    [
      ...npm.args,
      "pack",
      "--json",
      "--ignore-scripts",
      "--pack-destination",
      destination,
      packageRoot,
    ],
    { cwd: packageRoot, maxBuffer: 10 * 1024 * 1024 },
  );
  const reports = JSON.parse(stdout);
  if (!Array.isArray(reports) || reports.length !== 1)
    throw new Error("[factory-visualizers-pack] expected one npm pack report");
  const report = reports[0];
  const actualFiles = report.files
    .map(({ path: file }) => file.replaceAll("\\", "/"))
    .sort();
  const expectedFiles = [
    ...metadataFiles,
    ...(await listFiles(path.join(packageRoot, "dist"), "dist")),
  ].sort();
  if (JSON.stringify(actualFiles) !== JSON.stringify(expectedFiles)) {
    throw new Error(
      `[factory-visualizers-pack] unexpected tarball inventory\nactual: ${actualFiles.join(", ")}\nexpected: ${expectedFiles.join(", ")}`,
    );
  }
  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  for (const target of collectExportTargets(manifest.exports)) {
    if (!actualFiles.includes(target))
      throw new Error(
        `[factory-visualizers-pack] missing export target ${target}`,
      );
  }
  const styles = await readFile(
    path.join(packageRoot, "dist", "styles.css"),
    "utf8",
  );
  if (!styles.includes('@import "@xyflow/react/dist/style.css"')) {
    throw new Error(
      "[factory-visualizers-pack] packaged styles omit React Flow styles",
    );
  }
  const tarballPath = path.join(destination, report.filename);
  await access(tarballPath);
  return { files: actualFiles, packageName: report.name, tarballPath };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const temporaryDirectory = await mkdtemp(
    path.join(tmpdir(), "you-visualizers-pack-"),
  );
  try {
    const result = await packAndVerify(temporaryDirectory);
    process.stdout.write(
      `[factory-visualizers-pack] verified ${result.files.length} files\n`,
    );
  } finally {
    await rm(temporaryDirectory, { force: true, recursive: true });
  }
}
