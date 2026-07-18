import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const packageDirectory = fileURLToPath(new URL("../", import.meta.url));
const repositoryRoot = fileURLToPath(new URL("../../../", import.meta.url));
const clientPackage = join(repositoryRoot, "packages", "client");
const typescript = join(packageDirectory, "node_modules", "typescript", "bin", "tsc");
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";

function run(command, arguments_, { cwd, capture = false } = {}) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(command, arguments_, {
      cwd,
      shell: process.platform === "win32" && command === npmCommand,
      stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit",
    });
    let stdout = "";
    let stderr = "";
    child.stdout?.setEncoding("utf8");
    child.stderr?.setEncoding("utf8");
    child.stdout?.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr?.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", rejectPromise);
    child.on("close", (status) => {
      if (status !== 0) {
        rejectPromise(
          new Error(`${command} ${arguments_.join(" ")} failed with status ${status}\n${stderr}`),
        );
        return;
      }
      resolvePromise(stdout);
    });
  });
}

async function pack(source, destination) {
  const output = await run(
    npmCommand,
    [
      "pack",
      "--json",
      "--ignore-scripts",
      "--pack-destination",
      destination,
      source,
    ],
    { capture: true },
  );
  const [report] = JSON.parse(output);
  return join(destination, report.filename);
}

test("packed emulator resolves its canonical contract from a clean consumer", async (t) => {
  const temporaryRoot = await mkdtemp(
    join(tmpdir(), "you-factory-emulator-consumer-"),
  );
  t.after(() => rm(temporaryRoot, { recursive: true, force: true }));
  const packedDirectory = join(temporaryRoot, "packed");
  const consumerDirectory = join(temporaryRoot, "consumer");
  await mkdir(packedDirectory);
  await mkdir(consumerDirectory);

  const [clientTarball, emulatorTarball] = await Promise.all([
    pack(clientPackage, packedDirectory),
    pack(packageDirectory, packedDirectory),
  ]);
  await writeFile(
    join(consumerDirectory, "package.json"),
    `${JSON.stringify(
      {
        private: true,
        type: "module",
        dependencies: {
          "@you-agent-factory/client": `file:${clientTarball}`,
          "@you-agent-factory/factory-emulator": `file:${emulatorTarball}`,
        },
      },
      null,
      2,
    )}\n`,
  );
  await run(npmCommand, ["install", "--ignore-scripts", "--no-audit", "--no-fund"], {
    cwd: consumerDirectory,
  });

  await writeFile(
    join(consumerDirectory, "consumer.ts"),
    `import type { FactoryEvent } from "@you-agent-factory/client";
import { createMemoryFactoryEventSink } from "@you-agent-factory/factory-emulator";
import type {
  FactoryEventBatch,
  FactoryEventSink,
} from "@you-agent-factory/factory-emulator";

declare const event: FactoryEvent;
const batch: FactoryEventBatch = { events: [event] };
declare const sink: FactoryEventSink;
void sink.write(batch);
void createMemoryFactoryEventSink({ maxEvents: 1 });
`,
  );
  await writeFile(
    join(consumerDirectory, "tsconfig.json"),
    `${JSON.stringify(
      {
        compilerOptions: {
          exactOptionalPropertyTypes: true,
          module: "NodeNext",
          moduleResolution: "NodeNext",
          noEmit: true,
          strict: true,
          target: "ES2022",
        },
        include: ["consumer.ts"],
      },
      null,
      2,
    )}\n`,
  );
  await run(process.execPath, [typescript, "--project", "tsconfig.json"], {
    cwd: consumerDirectory,
  });

  const installedManifest = JSON.parse(
    await readFile(
      join(
        consumerDirectory,
        "node_modules",
        "@you-agent-factory",
        "factory-emulator",
        "package.json",
      ),
      "utf8",
    ),
  );
  assert.equal(installedManifest.dependencies, undefined);
  assert.equal(installedManifest.peerDependencies["@you-agent-factory/client"], "^0.0.0");
  const dependencyTree = JSON.parse(
    await run(npmCommand, ["ls", "--all", "--json"], {
      capture: true,
      cwd: consumerDirectory,
    }),
  );
  assert.equal(dependencyTree.problems, undefined);
});
