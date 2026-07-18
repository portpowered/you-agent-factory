import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { packAndVerify } from "./verify-package-pack.mjs";

const execFileAsync = promisify(execFile);
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

const TYPE_CONSUMER = `import {
  type components,
  type FactoryDefinition,
  type FactoryEvent,
  type operations,
  type paths,
  createFactoryEventCursor,
  orderFactoryEvents,
  parseFactoryEventReplayText,
  parseFactoryRecording,
} from "@you-agent-factory/client";

declare const input: unknown;
const recording = parseFactoryRecording(input);
const events: FactoryEvent[] = orderFactoryEvents(recording.events);
const factory: FactoryDefinition | undefined = recording.factory;
const cursor = events[0] ? createFactoryEventCursor(events[0]) : undefined;
const replay = parseFactoryEventReplayText("");
type ApiNamespaces = [components, paths, operations];
void factory;
void cursor;
void replay;
void (null as unknown as ApiNamespaces);
`;

const RUNTIME_CONSUMER = `import { readFile } from "node:fs/promises";
import {
  createFactoryEventCursor,
  orderFactoryEvents,
  parseFactoryEventReplayText,
  parseFactoryRecording,
} from "@you-agent-factory/client";

const exampleUrl = import.meta.resolve(
  "@you-agent-factory/client/examples/customer-support.factory-recording.v1.json",
);
const recording = parseFactoryRecording(
  JSON.parse(await readFile(new URL(exampleUrl), "utf8")),
);
const ordered = orderFactoryEvents([...recording.events].reverse());
const replayed = parseFactoryEventReplayText(
  ordered.map((event) => \`data: \${JSON.stringify(event)}\\n\\n\`).join(""),
);
if (replayed.length !== ordered.length || replayed[0]?.id !== ordered[0]?.id) {
  throw new Error("installed replay helpers returned inconsistent ordering");
}
const cursor = createFactoryEventCursor(ordered.at(-1));
if (!cursor.afterEventId || cursor.tick < 0) {
  throw new Error("installed cursor helper returned an invalid position");
}
process.stdout.write(\`verified \${recording.id} with \${ordered.length} events\\n\`);
`;

async function writeConsumer(consumerRoot, tarballPath) {
  const manifest = {
    name: "factory-client-installed-consumer",
    private: true,
    type: "module",
    dependencies: {
      "@you-agent-factory/client": pathToFileURL(tarballPath).href,
    },
  };
  await writeFile(
    path.join(consumerRoot, "package.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
  await writeFile(path.join(consumerRoot, "consumer.ts"), TYPE_CONSUMER);
  await writeFile(path.join(consumerRoot, "consumer.mjs"), RUNTIME_CONSUMER);
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
  path.join(tmpdir(), "you-client-consumer-"),
);
try {
  const packRoot = path.join(temporaryRoot, "pack");
  const consumerRoot = path.join(temporaryRoot, "consumer");
  await Promise.all([mkdir(packRoot), mkdir(consumerRoot)]);
  const packed = await packAndVerify(packRoot);
  await writeConsumer(consumerRoot, packed.tarballPath);

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
    { cwd: consumerRoot, maxBuffer: 10 * 1024 * 1024 },
  );
  process.stdout.write(`[client-installed-consumer] ${stdout}`);
} finally {
  await rm(temporaryRoot, { force: true, recursive: true });
}
