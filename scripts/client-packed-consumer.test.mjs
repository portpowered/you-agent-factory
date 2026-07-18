import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import {
  access,
  lstat,
  mkdtemp,
  mkdir,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../", import.meta.url));
const clientPackage = join(repositoryRoot, "packages", "client");
const apiPackage = join(repositoryRoot, "packages", "api");
const typescript = join(
  repositoryRoot,
  "ui",
  "node_modules",
  "typescript",
  "bin",
  "tsc",
);
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";

const reviewedInventories = new Map([
  [
    "@you-agent-factory/client",
    [
      "LICENSE.md",
      "README.md",
      "package.json",
      "recordings/factory-recording.json",
      "src/contracts.ts",
      "src/generated/factory-recording.schema.json",
      "src/generated/openapi.ts",
      "src/index.js",
      "src/index.ts",
      "src/recording-parser.d.ts",
      "src/recording-parser.js",
    ],
  ],
  [
    "@you-agent-factory/api",
    [
      "LICENSE.md",
      "README.md",
      "generated/cli/commands.json",
      "generated/javascript/runtime-api.json",
      "generated/joined/contracts/common/deprecations.schema.json",
      "generated/joined/contracts/common/documentation.schema.json",
      "generated/joined/contracts/manifest.schema.json",
      "generated/manifest.json",
      "generated/mcp/tools.json",
      "generated/openapi/openapi.yaml",
      "generated/schemas/factory-event.schema.json",
      "generated/schemas/factory-recording.schema.json",
      "generated/schemas/factory.schema.json",
      "generated/schemas/mock-workers.schema.json",
      "generated/schemas/you-config.schema.json",
      "package.json",
    ],
  ],
]);

const prohibitedDependencyPatterns = Object.freeze([
  /^react(?:-|$)/,
  /^zustand$/,
  /dashboard/,
  /(?:browser-)?storage/,
  /rout(?:e|er|ing)/,
  /api-error/,
]);

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
          new Error(
            `${command} ${arguments_.join(" ")} failed with status ${status}\n${stderr}`,
          ),
        );
        return;
      }
      resolvePromise(stdout);
    });
  });
}

async function pack(packageDirectory, destination) {
  const output = await run(
    npmCommand,
    [
      "pack",
      "--json",
      "--ignore-scripts",
      "--pack-destination",
      destination,
      packageDirectory,
    ],
    { capture: true },
  );
  const reports = JSON.parse(output);
  assert.equal(reports.length, 1);
  const report = reports[0];
  const actualFiles = report.files
    .map(({ path }) => path.replaceAll("\\", "/"))
    .sort((left, right) => left.localeCompare(right));
  const expectedFiles = [...reviewedInventories.get(report.name)].sort(
    (left, right) => left.localeCompare(right),
  );
  assert.deepEqual(actualFiles, expectedFiles);
  const tarball = join(destination, report.filename);
  await access(tarball);
  return tarball;
}

function collectDependencyNames(node, names = new Set()) {
  for (const [name, dependency] of Object.entries(node.dependencies ?? {})) {
    names.add(name);
    collectDependencyNames(dependency, names);
  }
  return names;
}

test("packed packages satisfy the recording contract from a clean consumer", async (t) => {
  const temporaryRoot = await mkdtemp(join(tmpdir(), "you-client-consumer-"));
  t.after(() => rm(temporaryRoot, { recursive: true, force: true }));
  const packedDirectory = join(temporaryRoot, "packed");
  const consumerDirectory = join(temporaryRoot, "consumer");
  await mkdir(packedDirectory);
  await mkdir(consumerDirectory);

  const clientTarball = await pack(clientPackage, packedDirectory);
  const apiTarball = await pack(apiPackage, packedDirectory);
  await writeFile(
    join(consumerDirectory, "package.json"),
    `${JSON.stringify(
      {
        private: true,
        type: "module",
        dependencies: {
          "@you-agent-factory/api": `file:${apiTarball}`,
          "@you-agent-factory/client": `file:${clientTarball}`,
        },
      },
      null,
      2,
    )}\n`,
  );
  await run(
    npmCommand,
    ["install", "--ignore-scripts", "--no-audit", "--no-fund"],
    { cwd: consumerDirectory },
  );

  await writeFile(
    join(consumerDirectory, "consumer.ts"),
    `import type {
  FactoryDefinition,
  FactoryEvent,
  FactoryEventType,
  FactoryRecording,
  components,
  operations,
  paths,
} from "@you-agent-factory/client";
import {
  parseFactoryRecording,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";

declare const input: unknown;
const parsed: FactoryRecording = parseFactoryRecording(input);
const safe = safeParseFactoryRecording(input);
const event: FactoryEvent = parsed.events[0];
const eventType: FactoryEventType = event.type;
type Factory = FactoryDefinition;
type Components = components;
type Paths = paths;
type Operations = operations;
const evidence: unknown[] = [safe, eventType];
void evidence;
export type { Components, Factory, Operations, Paths };
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

  await writeFile(
    join(consumerDirectory, "verify.mjs"),
    `import assert from "node:assert/strict";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import factoryEventSchema from "@you-agent-factory/api/schemas/factory-event" with { type: "json" };
import factoryRecordingSchema from "@you-agent-factory/api/schemas/factory-recording" with { type: "json" };
import {
  parseFactoryRecording,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";
import recording from "@you-agent-factory/client/recordings/example" with { type: "json" };

const before = JSON.stringify(recording);
const ajv = new Ajv2020({ strict: false });
addFormats(ajv);
assert.equal(ajv.compile(factoryEventSchema)(recording.events[0]), true);
assert.equal(ajv.compile(factoryRecordingSchema)(recording), true);
assert.equal(parseFactoryRecording(recording), recording);
const result = safeParseFactoryRecording(recording);
assert.equal(result.success, true);
assert.equal(result.data, recording);
assert.equal(JSON.stringify(recording), before);
`,
  );
  await run(process.execPath, ["verify.mjs"], { cwd: consumerDirectory });

  const dependencyTree = JSON.parse(
    await run(npmCommand, ["ls", "--all", "--json"], {
      capture: true,
      cwd: consumerDirectory,
    }),
  );
  const installedNames = collectDependencyNames(dependencyTree);
  for (const installedName of installedNames) {
    for (const prohibited of prohibitedDependencyPatterns) {
      assert.doesNotMatch(installedName.toLowerCase(), prohibited);
    }
  }

  const installedClient = join(
    consumerDirectory,
    "node_modules",
    "@you-agent-factory",
    "client",
  );
  const installedApi = join(
    consumerDirectory,
    "node_modules",
    "@you-agent-factory",
    "api",
  );
  assert.equal((await lstat(installedClient)).isSymbolicLink(), false);
  assert.equal((await lstat(installedApi)).isSymbolicLink(), false);
  assert.notEqual(await realpath(installedClient), resolve(clientPackage));
  assert.notEqual(await realpath(installedApi), resolve(apiPackage));
  assert.equal(
    JSON.parse(await readFile(join(installedClient, "package.json"), "utf8"))
      .name,
    "@you-agent-factory/client",
  );
});
