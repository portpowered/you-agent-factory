import { execFile } from "node:child_process";
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { packAndVerify as packClient } from "../../client/scripts/verify-package-pack.mjs";
import { packAndVerify as packEmulator } from "./verify-package-pack.mjs";

const execFileAsync = promisify(execFile);
const require = createRequire(import.meta.url);
const packageRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const clientRoot = path.resolve(packageRoot, "../client");

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

async function prepareClientPackage(npm) {
  const clientNodeModules = path.join(clientRoot, "node_modules");
  try {
    await access(path.join(clientNodeModules, "typescript", "package.json"));
    return undefined;
  } catch {
    await execFileAsync(
      npm.executable,
      [
        ...npm.args,
        "install",
        "--ignore-scripts",
        "--no-audit",
        "--no-fund",
        "--package-lock=false",
      ],
      { cwd: clientRoot, maxBuffer: 10 * 1024 * 1024 },
    );
    return clientNodeModules;
  }
}

const typeConsumer = `import type { FactoryDefinition, FactoryEvent } from "@you-agent-factory/client";
import {
  type FactoryEmulatorScenario,
  type FactoryEventSink,
  MemoryFactoryEventSink,
  RecordingFactoryEventSink,
  inspectFactoryEmulatorCompatibility,
  parseFactoryEmulatorScenario,
  scenarioSchema,
} from "@you-agent-factory/factory-emulator";

declare const factory: FactoryDefinition;
declare const event: FactoryEvent;
declare const input: unknown;
const scenario: FactoryEmulatorScenario = parseFactoryEmulatorScenario(input, factory);
const sink: FactoryEventSink = new MemoryFactoryEventSink({ maxEvents: 1 });
const recordingSink = new RecordingFactoryEventSink({
  maxEvents: 1,
  recording: {
    schemaVersion: "factory-recording/v1",
    id: "fixture",
    title: "Fixture",
  },
});
void event;
void scenario;
void sink;
void recordingSink;
void scenarioSchema;
void inspectFactoryEmulatorCompatibility(factory);
`;

const runtimeConsumer = `import { readFile } from "node:fs/promises";
import { safeParseFactoryRecording } from "@you-agent-factory/client";
import {
  MemoryFactoryEventSink,
  RecordingFactoryEventSink,
  inspectFactoryEmulatorCompatibility,
  parseFactoryEmulatorScenario,
  scenarioSchema,
} from "@you-agent-factory/factory-emulator";

const factory = {
  name: "customer-support",
  workTypes: [{
    name: "ticket",
    states: [
      { name: "new", type: "INITIAL" },
      { name: "done", type: "TERMINAL" },
      { name: "failed", type: "FAILED" },
    ],
  }],
  workers: [{ name: "support-agent", type: "AGENT_WORKER" }],
  workstations: [{
    name: "triage",
    type: "AGENT_RUN",
    behavior: "STANDARD",
    worker: "support-agent",
    inputs: [{ workType: "ticket", state: "new" }],
    outputs: [{ workType: "ticket", state: "done" }],
    onFailure: [{ workType: "ticket", state: "failed" }],
  }],
};
const exampleUrl = import.meta.resolve(
  "@you-agent-factory/factory-emulator/examples/customer-support.scenario.v1.json",
);
const schemaUrl = import.meta.resolve("@you-agent-factory/factory-emulator/schema");
const scenario = parseFactoryEmulatorScenario(
  JSON.parse(await readFile(new URL(exampleUrl), "utf8")),
  factory,
);
const installedSchema = JSON.parse(await readFile(new URL(schemaUrl), "utf8"));
if (scenario.id !== "customer-support-happy-path" || installedSchema.$id !== scenarioSchema.$id) {
  throw new Error("installed scenario exports are inconsistent");
}
if (!inspectFactoryEmulatorCompatibility(factory).supported) {
  throw new Error("installed compatibility inspector rejected the supported Factory");
}

const events = [
  {
    schemaVersion: "agent-factory.event.v1",
    id: "event-1",
    type: "INITIAL_STRUCTURE_REQUEST",
    context: {
      sequence: 1,
      sessionSequence: 1,
      tick: 1,
      eventTime: "2026-07-18T16:00:00Z",
      sessionId: "session-1",
    },
    payload: { factory },
  },
  {
    schemaVersion: "agent-factory.event.v1",
    id: "event-2",
    type: "SESSION_STARTED",
    context: {
      sequence: 2,
      sessionSequence: 2,
      tick: 2,
      eventTime: "2026-07-18T16:00:01Z",
      sessionId: "session-1",
    },
    payload: { startedAt: "2026-07-18T16:00:00Z" },
  },
];
const memory = new MemoryFactoryEventSink({ maxEvents: 2 });
await memory.write(events);
const recording = new RecordingFactoryEventSink({
  maxEvents: 2,
  recording: {
    schemaVersion: "factory-recording/v1",
    id: "installed-consumer-recording",
    title: "Installed consumer recording",
    factory,
  },
});
await recording.write(events);
const snapshot = recording.snapshot();
if (memory.snapshot().length !== 2 || !safeParseFactoryRecording(snapshot).success) {
  throw new Error("installed sinks did not produce valid retained history");
}
await Promise.all([memory.close(), recording.close()]);
process.stdout.write(\`verified \${scenario.id} with \${snapshot.events.length} events\n\`);
`;

async function writeConsumer(consumerRoot, clientTarball, emulatorTarball) {
  const manifest = {
    name: "factory-emulator-installed-consumer",
    private: true,
    type: "module",
    dependencies: {
      "@you-agent-factory/client": pathToFileURL(clientTarball).href,
      "@you-agent-factory/factory-emulator":
        pathToFileURL(emulatorTarball).href,
    },
  };
  await writeFile(
    path.join(consumerRoot, "package.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
  await writeFile(path.join(consumerRoot, "consumer.ts"), typeConsumer);
  await writeFile(path.join(consumerRoot, "consumer.mjs"), runtimeConsumer);
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
}

const temporaryRoot = await mkdtemp(
  path.join(tmpdir(), "factory-emulator-consumer-"),
);
let preparedClientNodeModules;
try {
  const clientPackRoot = path.join(temporaryRoot, "client-pack");
  const emulatorPackRoot = path.join(temporaryRoot, "emulator-pack");
  const consumerRoot = path.join(temporaryRoot, "consumer");
  await Promise.all([
    mkdir(clientPackRoot),
    mkdir(emulatorPackRoot),
    mkdir(consumerRoot),
  ]);
  const npm = await npmCommand();
  preparedClientNodeModules = await prepareClientPackage(npm);
  const client = await packClient(clientPackRoot);
  const emulator = await packEmulator(emulatorPackRoot);
  await writeConsumer(consumerRoot, client.tarballPath, emulator.tarballPath);

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
    { cwd: consumerRoot, maxBuffer: 10 * 1024 * 1024 },
  );
  process.stdout.write(`[factory-emulator-consumer] ${stdout}`);
} finally {
  await rm(temporaryRoot, { force: true, recursive: true });
  if (preparedClientNodeModules !== undefined) {
    await rm(preparedClientNodeModules, { force: true, recursive: true });
  }
}
