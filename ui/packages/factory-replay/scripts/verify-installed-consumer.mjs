import { execFile } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { packAndVerify as packClient } from "../../client/scripts/verify-package-pack.mjs";
import { packAndVerify as packReplay } from "./verify-package-pack.mjs";

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const require = createRequire(import.meta.url);

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

const temporaryRoot = await mkdtemp(
  path.join(tmpdir(), "you-replay-consumer-"),
);
try {
  const clientPackRoot = path.join(temporaryRoot, "client-pack");
  const replayPackRoot = path.join(temporaryRoot, "replay-pack");
  const consumerRoot = path.join(temporaryRoot, "consumer");
  await Promise.all([
    mkdir(clientPackRoot),
    mkdir(replayPackRoot),
    mkdir(consumerRoot),
  ]);
  // The replay build resolves the client's generated declarations, so finish
  // the client pack before rebuilding replay instead of racing both builds.
  const client = await packClient(clientPackRoot);
  const replay = await packReplay(replayPackRoot);
  await Promise.all([
    cp(
      path.join(packageRoot, "fixtures", "installed-consumer", "consumer.ts"),
      path.join(consumerRoot, "consumer.ts"),
    ),
    cp(
      path.join(packageRoot, "fixtures", "installed-consumer", "consumer.mjs"),
      path.join(consumerRoot, "consumer.mjs"),
    ),
  ]);
  await writeFile(
    path.join(consumerRoot, "package.json"),
    `${JSON.stringify(
      {
        name: "factory-replay-installed-consumer",
        private: true,
        type: "module",
        dependencies: {
          "@you-agent-factory/client": pathToFileURL(client.tarballPath).href,
          "@you-agent-factory/factory-replay": pathToFileURL(replay.tarballPath)
            .href,
        },
      },
      null,
      2,
    )}\n`,
  );
  await writeFile(
    path.join(consumerRoot, "tsconfig.json"),
    `${JSON.stringify(
      {
        compilerOptions: {
          module: "NodeNext",
          moduleResolution: "NodeNext",
          noEmit: true,
          strict: true,
          target: "ES2022",
        },
        files: ["consumer.ts"],
      },
      null,
      2,
    )}\n`,
  );
  const npm = await npmCommand();
  await execFileAsync(
    npm.executable,
    [...npm.args, "install", "--ignore-scripts", "--no-audit", "--no-fund"],
    { cwd: consumerRoot, maxBuffer: 10 * 1024 * 1024 },
  );
  const typescriptManifestPath = require.resolve("typescript/package.json");
  const typescriptManifest = JSON.parse(
    await readFile(typescriptManifestPath, "utf8"),
  );
  const typescriptBin = path.resolve(
    path.dirname(typescriptManifestPath),
    typescriptManifest.bin.tsc,
  );
  await execFileAsync(process.execPath, [typescriptBin, "--pretty", "false"], {
    cwd: consumerRoot,
    maxBuffer: 10 * 1024 * 1024,
  });
  const { stdout } = await execFileAsync(
    process.execPath,
    [path.join(consumerRoot, "consumer.mjs")],
    {
      cwd: consumerRoot,
      maxBuffer: 10 * 1024 * 1024,
    },
  );
  const { stdout: dependencyTree } = await execFileAsync(
    npm.executable,
    [...npm.args, "ls", "--all", "--json"],
    { cwd: consumerRoot, maxBuffer: 10 * 1024 * 1024 },
  );
  const installed = JSON.parse(dependencyTree).dependencies ?? {};
  if (
    !installed["@you-agent-factory/client"] ||
    !installed["@you-agent-factory/factory-replay"]
  ) {
    throw new Error(
      "[factory-replay-consumer] packed dependencies were not installed",
    );
  }
  process.stdout.write(`[factory-replay-consumer] ${stdout}`);
} finally {
  await rm(temporaryRoot, { force: true, recursive: true });
}
