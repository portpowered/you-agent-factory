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

async function runPackageBin(packageName, binName, args) {
  const manifestPath = require.resolve(`${packageName}/package.json`);
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const binPath =
    typeof manifest.bin === "string" ? manifest.bin : manifest.bin?.[binName];
  if (!binPath) throw new Error(`${packageName} does not declare ${binName}`);
  await execFileAsync(
    process.execPath,
    [path.resolve(path.dirname(manifestPath), binPath), ...args],
    {
      cwd: packageRoot,
      maxBuffer: 10 * 1024 * 1024,
    },
  );
}

for (const dependency of [
  "client",
  "factory-replay",
  "components",
  "factory-graph",
]) {
  await execFileAsync(
    process.execPath,
    [
      path.resolve(
        packageRoot,
        "..",
        dependency,
        "scripts",
        "build-package.mjs",
      ),
    ],
    {
      cwd: packageRoot,
      maxBuffer: 10 * 1024 * 1024,
    },
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
await cp(
  path.join(packageRoot, "src", "styles.css"),
  path.join(distRoot, "styles.css"),
);
