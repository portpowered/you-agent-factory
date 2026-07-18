import { execFile } from "node:child_process";
import { readFile, rm } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const require = createRequire(import.meta.url);
const typescriptManifestPath = require.resolve("typescript/package.json");
const typescriptManifest = JSON.parse(
  await readFile(typescriptManifestPath, "utf8"),
);
const typescriptBin = path.resolve(
  path.dirname(typescriptManifestPath),
  typescriptManifest.bin.tsc,
);
const dist = path.join(packageRoot, "dist");

await rm(dist, { force: true, recursive: true });
await execFileAsync(
  process.execPath,
  [typescriptBin, "--project", "tsconfig.build.json", "--pretty", "false"],
  { cwd: packageRoot, maxBuffer: 10 * 1024 * 1024 },
);
