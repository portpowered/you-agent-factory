import { execFile } from "node:child_process";
import { access, mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

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
    { cwd: packageRoot, maxBuffer: 20 * 1024 * 1024 },
  );
  await execFileAsync(
    process.execPath,
    [path.join(packageRoot, "scripts", "check-package-boundary.mjs")],
    { cwd: packageRoot, maxBuffer: 20 * 1024 * 1024 },
  );
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
    throw new Error("[factory-graph-pack] expected one npm pack report");
  const report = reports[0];
  const actualFiles = report.files
    .map(({ path: file }) => file.replaceAll("\\", "/"))
    .sort();
  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  for (const target of collectExportTargets(manifest.exports)) {
    if (!actualFiles.includes(target))
      throw new Error(`[factory-graph-pack] missing export target ${target}`);
  }
  const tarballPath = path.join(destination, report.filename);
  await access(tarballPath);
  return { files: actualFiles, packageName: report.name, tarballPath };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const temporaryDirectory = await mkdtemp(
    path.join(tmpdir(), "you-factory-graph-pack-"),
  );
  try {
    const result = await packAndVerify(temporaryDirectory);
    process.stdout.write(
      `[factory-graph-pack] verified ${result.files.length} files\n`,
    );
  } finally {
    await rm(temporaryDirectory, { force: true, recursive: true });
  }
}
