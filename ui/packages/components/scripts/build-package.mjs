import { execFile } from "node:child_process";
import { cp, mkdir, readFile, rm } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const distRoot = path.join(packageRoot, "dist");
const require = createRequire(import.meta.url);
const sourceRoot = path.join(packageRoot, "src");

async function runPackageBin(packageName, binName, args) {
  const packageJsonPath = require.resolve(`${packageName}/package.json`);
  const packageJson = JSON.parse(await readFile(packageJsonPath, "utf8"));
  const binPath =
    typeof packageJson.bin === "string"
      ? packageJson.bin
      : packageJson.bin?.[binName];
  if (!binPath) {
    throw new Error(
      `${packageName} does not declare the ${binName} executable`,
    );
  }

  try {
    await execFileAsync(
      process.execPath,
      [path.resolve(path.dirname(packageJsonPath), binPath), ...args],
      {
        cwd: packageRoot,
        maxBuffer: 10 * 1024 * 1024,
      },
    );
  } catch (error) {
    if (error.stdout) process.stdout.write(error.stdout);
    if (error.stderr) process.stderr.write(error.stderr);
    throw error;
  }
}

async function copyStyleDependency(sourcePath, copiedPaths = new Set()) {
  const absoluteSourcePath = path.resolve(sourcePath);
  if (copiedPaths.has(absoluteSourcePath)) return;
  if (!absoluteSourcePath.startsWith(`${sourceRoot}${path.sep}`)) {
    throw new Error(
      `Stylesheet dependency escapes the package source: ${sourcePath}`,
    );
  }

  copiedPaths.add(absoluteSourcePath);
  const relativePath = path.relative(sourceRoot, absoluteSourcePath);
  const outputPath = path.join(distRoot, relativePath);
  await mkdir(path.dirname(outputPath), { recursive: true });
  await cp(absoluteSourcePath, outputPath);

  if (path.extname(absoluteSourcePath) !== ".css") return;
  const css = await readFile(absoluteSourcePath, "utf8");
  const referencedPaths = [
    ...css.matchAll(/@import\s+["'](.+?)["']/g),
    ...css.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/g),
  ].map((match) => match[1]);

  await Promise.all(
    referencedPaths
      .filter(
        (referencedPath) =>
          referencedPath.startsWith(".") &&
          !/^(?:[a-z]+:|data:|#)/i.test(referencedPath),
      )
      .map((referencedPath) =>
        copyStyleDependency(
          path.resolve(
            path.dirname(absoluteSourcePath),
            referencedPath.split(/[?#]/, 1)[0],
          ),
          copiedPaths,
        ),
      ),
  );
}

await rm(distRoot, { force: true, recursive: true });
await mkdir(distRoot, { recursive: true });

await runPackageBin("vite", "vite", [
  "build",
  "--config",
  "vite.build.config.ts",
]);
await runPackageBin("typescript", "tsc", [
  "--project",
  "tsconfig.build.json",
  "--pretty",
  "false",
]);

await copyStyleDependency(path.join(sourceRoot, "styles.css"));
