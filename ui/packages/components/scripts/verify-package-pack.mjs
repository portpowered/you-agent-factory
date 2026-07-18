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
const defaultPackageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const REQUIRED_METADATA_FILES = ["LICENSE.md", "README.md", "package.json"];
const FORBIDDEN_PATH_SEGMENTS = new Set([
  ".storybook",
  "docs",
  "fixtures",
  "node_modules",
  "scripts",
  "src",
  "storybook-static",
  "stories",
  "tests",
]);

function normalizePackPath(filePath) {
  return filePath.replaceAll("\\", "/").replace(/^package\//, "");
}

function sortedUnique(filePaths) {
  return [...new Set(filePaths.map(normalizePackPath))].sort((left, right) =>
    left.localeCompare(right),
  );
}

async function listFilesRecursively(directory, relativeRoot = "") {
  const entries = await readdir(directory, { withFileTypes: true });
  const nestedFiles = await Promise.all(
    entries.map((entry) => {
      const relativePath = path.posix.join(relativeRoot, entry.name);
      return entry.isDirectory()
        ? listFilesRecursively(path.join(directory, entry.name), relativePath)
        : relativePath;
    }),
  );
  return nestedFiles.flat();
}

function collectExportTargets(exports, exportPath = "exports") {
  if (typeof exports === "string") {
    return [
      { exportPath, target: normalizePackPath(exports.replace(/^\.\//, "")) },
    ];
  }
  if (!exports || typeof exports !== "object") return [];

  return Object.entries(exports).flatMap(([condition, target]) =>
    collectExportTargets(target, `${exportPath}[${JSON.stringify(condition)}]`),
  );
}

function referencedStylePaths(styles) {
  return [
    ...styles.matchAll(/@import\s+["'](.+?)["']/g),
    ...styles.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/g),
  ].map((match) => match[1]);
}

function isForbiddenPackPath(filePath) {
  const segments = filePath.split("/");
  if (segments.some((segment) => FORBIDDEN_PATH_SEGMENTS.has(segment))) {
    return true;
  }
  return (
    /(?:^|\/)bun\.lock(?:b)?$/.test(filePath) ||
    /(?:^|\/)(?:tsconfig|vite|vitest)[^/]*\.(?:js|mjs|ts|json)$/.test(
      filePath,
    ) ||
    /(?:^|\/)(?![^/]*\.d\.ts$)[^/]+\.tsx?$/.test(filePath) ||
    /(?:^|\/)[^/]+\.(?:test|stories)\.[^/]+$/.test(filePath)
  );
}

function formatInventoryFailure(diagnostics) {
  return new Error(
    [
      "[components-package-pack] tarball inventory rejected",
      ...diagnostics,
    ].join("\n"),
  );
}

export async function validatePackedInventory({
  files,
  manifest,
  packageFiles,
  readPackedFile,
}) {
  const actualFiles = sortedUnique(files);
  const expectedFiles = sortedUnique(packageFiles);
  const actualSet = new Set(actualFiles);
  const expectedSet = new Set(expectedFiles);
  const diagnostics = [];

  const unexpectedFiles = actualFiles.filter((file) => !expectedSet.has(file));
  const missingFiles = expectedFiles.filter((file) => !actualSet.has(file));
  const forbiddenFiles = actualFiles.filter(isForbiddenPackPath);

  if (unexpectedFiles.length > 0) {
    diagnostics.push(
      "unexpected package files:",
      ...unexpectedFiles.map((file) => `  ${file}`),
    );
  }
  if (missingFiles.length > 0) {
    diagnostics.push(
      "missing package files:",
      ...missingFiles.map((file) => `  ${file}`),
    );
  }
  if (forbiddenFiles.length > 0) {
    diagnostics.push(
      "development-only files must not be published:",
      ...forbiddenFiles.map((file) => `  ${file}`),
    );
  }

  const exportTargets = collectExportTargets(manifest.exports);
  for (const { exportPath, target } of exportTargets) {
    if (!actualSet.has(target)) {
      diagnostics.push(`missing export target ${exportPath}: ${target}`);
    }
  }

  const pendingStylesheets = exportTargets
    .map(({ target }) => target)
    .filter((target) => target.endsWith(".css") && actualSet.has(target));
  const visitedStylesheets = new Set();
  while (pendingStylesheets.length > 0) {
    const stylesheetPath = pendingStylesheets.pop();
    if (!stylesheetPath || visitedStylesheets.has(stylesheetPath)) continue;
    visitedStylesheets.add(stylesheetPath);

    const styles = await readPackedFile(stylesheetPath);
    for (const referencedPath of referencedStylePaths(styles)) {
      if (
        !referencedPath.startsWith(".") ||
        /^(?:[a-z]+:|data:|#)/i.test(referencedPath)
      ) {
        continue;
      }
      const resolvedPath = path.posix.normalize(
        path.posix.join(
          path.posix.dirname(stylesheetPath),
          referencedPath.split(/[?#]/, 1)[0],
        ),
      );
      if (!actualSet.has(resolvedPath)) {
        diagnostics.push(
          `missing stylesheet dependency from ${stylesheetPath}: ${resolvedPath}`,
        );
      } else if (resolvedPath.endsWith(".css")) {
        pendingStylesheets.push(resolvedPath);
      }
    }
  }

  if (diagnostics.length > 0) throw formatInventoryFailure(diagnostics);
  return actualFiles;
}

async function runBuild(packageRoot) {
  try {
    await execFileAsync(
      process.execPath,
      [path.join(packageRoot, "scripts", "build-package.mjs")],
      { cwd: packageRoot, maxBuffer: 10 * 1024 * 1024 },
    );
  } catch (error) {
    throw new Error(
      `[components-package-pack] package build failed\n${error.stderr?.trim() ?? error.message}`,
      { cause: error },
    );
  }
}

async function runNpmPack(packageRoot, packDestination) {
  let npmExecutable = "npm";
  let npmArguments = [];
  if (process.platform === "win32") {
    const { stdout } = await execFileAsync("where.exe", ["npm.cmd"]);
    const npmCommand = stdout.trim().split(/\r?\n/, 1)[0];
    npmExecutable = process.execPath;
    npmArguments = [
      path.join(
        path.dirname(npmCommand),
        "node_modules",
        "npm",
        "bin",
        "npm-cli.js",
      ),
    ];
  }

  try {
    const { stdout } = await execFileAsync(
      npmExecutable,
      [
        ...npmArguments,
        "pack",
        "--json",
        "--ignore-scripts",
        "--pack-destination",
        packDestination,
        packageRoot,
      ],
      { cwd: packageRoot, maxBuffer: 10 * 1024 * 1024 },
    );
    return stdout;
  } catch (error) {
    throw new Error(
      `[components-package-pack] npm pack failed\n${error.stderr?.trim() ?? error.message}`,
      { cause: error },
    );
  }
}

export async function packAndVerify({
  packageDirectory = defaultPackageRoot,
  packDestination,
  build = true,
}) {
  const packageRoot = path.resolve(packageDirectory);
  const destination = path.resolve(packDestination);
  await mkdir(destination, { recursive: true });
  if (build) await runBuild(packageRoot);

  const stdout = await runNpmPack(packageRoot, destination);
  let reports;
  try {
    reports = JSON.parse(stdout);
  } catch (error) {
    throw new Error("[components-package-pack] npm pack did not return JSON", {
      cause: error,
    });
  }
  if (!Array.isArray(reports) || reports.length !== 1) {
    throw new Error(
      `[components-package-pack] npm pack returned ${reports?.length ?? "invalid"} reports, want 1`,
    );
  }

  const report = reports[0];
  const files = report.files?.map((file) => file.path);
  if (!Array.isArray(files) || files.some((file) => typeof file !== "string")) {
    throw new Error(
      "[components-package-pack] npm pack report has no valid file inventory",
    );
  }
  if (typeof report.filename !== "string" || report.filename.length === 0) {
    throw new Error(
      "[components-package-pack] npm pack report has no valid filename",
    );
  }

  const manifest = JSON.parse(
    await readFile(path.join(packageRoot, "package.json"), "utf8"),
  );
  const packageFiles = [
    ...REQUIRED_METADATA_FILES,
    ...(await listFilesRecursively(path.join(packageRoot, "dist"), "dist")),
  ];
  const verifiedFiles = await validatePackedInventory({
    files,
    manifest,
    packageFiles,
    readPackedFile: (file) => readFile(path.join(packageRoot, file), "utf8"),
  });
  const tarballPath = path.join(destination, report.filename);
  await access(tarballPath);

  return {
    files: verifiedFiles,
    packageName: report.name,
    packageVersion: report.version,
    tarballPath,
  };
}

async function main() {
  const temporaryDirectory = await mkdtemp(
    path.join(tmpdir(), "you-components-pack-"),
  );
  try {
    const result = await packAndVerify({ packDestination: temporaryDirectory });
    process.stdout.write(
      `[components-package-pack] verified ${result.files.length} files in ${path.basename(result.tarballPath)}\n`,
    );
  } finally {
    await rm(temporaryDirectory, { force: true, recursive: true });
  }
}

if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
) {
  await main();
}
