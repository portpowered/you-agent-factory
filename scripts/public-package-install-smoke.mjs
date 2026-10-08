import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { createHash } from "node:crypto";
import {
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { timePackagePhase } from "./package-phase-timing.mjs";
import { assertSourceCommit } from "./package-release-candidate.mjs";
import {
  assertCandidateSetEvidence,
  FRONTEND_ONLY_CANDIDATE_SCOPE,
} from "./public-package-set.mjs";

export async function validateFrontendTarballs(evidence, candidateDirectory) {
  assertCandidateSetEvidence(evidence, FRONTEND_ONLY_CANDIDATE_SCOPE);
  const candidates = [];
  const root = await realpath(candidateDirectory);
  for (const candidate of evidence.packages) {
    if (
      typeof candidate.filename !== "string" ||
      path.basename(candidate.filename) !== candidate.filename ||
      !candidate.filename.endsWith(".tgz") ||
      /[\\/]/.test(candidate.filename)
    ) {
      throw new Error(`Invalid candidate filename for ${candidate.name}`);
    }
    const tarball = await realpath(path.join(root, candidate.filename));
    if (path.dirname(tarball) !== root)
      throw new Error(`Candidate escapes directory: ${candidate.name}`);
    const bytes = await readFile(tarball);
    if (
      createHash("sha1").update(bytes).digest("hex") !== candidate.shasum ||
      `sha512-${createHash("sha512").update(bytes).digest("base64")}` !==
        candidate.integrity
    ) {
      throw new Error(`Candidate digest mismatch for ${candidate.name}`);
    }
    candidates.push({ ...candidate, tarball });
  }
  return candidates;
}

let nodeExecutable;
export function consumerNodeExecutable() {
  if (nodeExecutable) return nodeExecutable;
  // `bun run --bun` prepends a Node shim. Consumer proof must use real Node.
  const pathValue =
    Object.entries(process.env).find(
      ([key]) => key.toUpperCase() === "PATH",
    )?.[1] ?? "";
  for (const directory of pathValue.split(path.delimiter)) {
    const candidate = path.join(
      directory,
      process.platform === "win32" ? "node.exe" : "node",
    );
    if (!existsSync(candidate)) continue;
    const result = spawnSync(
      candidate,
      ["-p", "process.versions.bun ? '' : process.execPath"],
      { encoding: "utf8", windowsHide: true, timeout: 10_000 },
    );
    if (result.status === 0 && result.stdout.trim()) {
      nodeExecutable = result.stdout.trim();
      return nodeExecutable;
    }
  }
  throw new Error(
    "A real Node executable is required for installed consumer verification",
  );
}

export function runSmokeCommand(
  command,
  args,
  { cwd, env = process.env, timeoutMs = 120_000 } = {},
) {
  return new Promise((resolve, reject) => {
    const child = spawn(
      command === "node" ? consumerNodeExecutable() : command,
      args,
      {
        cwd,
        env,
        shell: false,
        windowsHide: true,
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    let stdout = "";
    let stderr = "";
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      child.kill();
    }, timeoutMs);
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.once("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.once("close", (code, signal) => {
      clearTimeout(timer);
      if (timedOut)
        reject(new Error(`${command} timed out after ${timeoutMs}ms`));
      else if (code === 0) resolve({ stdout, stderr });
      else
        reject(new Error(`${command} failed (${signal ?? code}): ${stderr}`));
    });
  });
}

// This source executes under Node from an isolated installed consumer, never a workspace.
export const frontendConsumerSource = `
import assert from "node:assert/strict";
import { readFileSync, realpathSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
const specs = JSON.parse(readFileSync("./expected.json", "utf8"));
const sample = JSON.parse(readFileSync(fileURLToPath(import.meta.resolve(
  "@you-agent-factory/factory-visualizers/examples/support-playback.factory-recording.v1.json")), "utf8"));
for (const spec of specs) {
  try {
  const root = realpathSync(path.join("node_modules", spec.name));
  assert.ok(root.startsWith(realpathSync(".") + path.sep), spec.name + " must be installed locally");
  const manifest = JSON.parse(readFileSync(path.join(root, "package.json"), "utf8"));
  assert.equal(manifest.name, spec.name);
  assert.equal(manifest.version, spec.version);
  const dataPackage = ["@you-agent-factory/api", "@you-agent-factory/packaged-factories"].includes(spec.name);
  const entry = dataPackage ? spec.name + "/manifest" : spec.name;
  assert.ok(realpathSync(fileURLToPath(import.meta.resolve(entry))).startsWith(root + path.sep));
  const api = dataPackage ? null : await import(spec.name);
  switch (spec.name) {
    case "@you-agent-factory/api": {
      const manifest = JSON.parse(readFileSync(fileURLToPath(import.meta.resolve(entry)), "utf8"));
      assert.equal(manifest.sourceCommit, spec.sourceCommit);
      const schema = JSON.parse(readFileSync(fileURLToPath(import.meta.resolve(spec.name + "/schemas/factory")), "utf8"));
      assert.ok(schema.properties);
      const openapi = readFileSync(fileURLToPath(import.meta.resolve(spec.name + "/openapi")), "utf8");
      assert.match(openapi, /openapi: 3\\./); break;
    }
    case "@you-agent-factory/packaged-factories": {
      const manifest = JSON.parse(readFileSync(fileURLToPath(import.meta.resolve(entry)), "utf8"));
      assert.equal(manifest.sourceCommit, spec.sourceCommit);
      assert.ok(manifest.factories.length > 0);
      const factory = JSON.parse(readFileSync(fileURLToPath(import.meta.resolve(
        spec.name + "/factories/" + manifest.factories[0].slug + ".json")), "utf8"));
      assert.ok(factory.name);
      const schema = JSON.parse(readFileSync(fileURLToPath(import.meta.resolve(spec.name + "/schemas/factory.json")), "utf8"));
      assert.ok(schema.properties); break;
    }
    case "@you-agent-factory/client": {
      const recording = api.parseFactoryRecording(sample);
      assert.deepEqual(api.orderFactoryEvents([...recording.events].reverse()).map(({ id }) => id),
        ["support-topology", "support-work"]); break;
    }
    case "@you-agent-factory/factory-replay": {
      const progress = api.projectFactoryWorkProgressAtTick({ events: sample.events, tick: 2 });
      assert.equal(progress.counts.queued, 1);
      assert.equal(progress.total, 1); break;
    }
    case "@you-agent-factory/factory-emulator": {
      const factory = { name: "release-smoke", orchestrator: { kind: "PETRI" }, workTypes: [
        { name: "task", states: [{ name: "ready", type: "INITIAL" }] } ] };
      const scenario = api.parseFactoryEmulatorScenario({ schemaVersion: "factory-emulator-scenario/v1",
        id: "release-smoke", factory: { name: factory.name }, seed: "release", startAt: "2026-07-18T16:00:00.000Z",
        rules: [], unmatched: { behavior: "error" } }, factory);
      const sink = new api.MemoryFactoryEventSink({ maxEvents: 100 });
      const session = api.createFactoryEmulatorSession({ factory, scenario, sink });
      assert.equal((await session.start()).status, "started");
      assert.ok(sink.snapshot().some(({ type }) => type === "SESSION_STARTED"));
      await session.close(); break;
    }
    case "@you-agent-factory/components":
      assert.match(renderToStaticMarkup(React.createElement(api.Button, {}, "Release smoke")), /Release smoke/); break;
    case "@you-agent-factory/factory-graph":
      assert.equal(api.factoryGraphNodeFamilyRole("worker").family, "worker"); break;
    case "@you-agent-factory/factory-visualizers": {
      const counts = { active: 0, completed: 0, failed: 0, queued: 0, unclassified: 0 };
      const html = renderToStaticMarkup(React.createElement(api.WorkProgressVisualizer, {
        formatNumber: String, projection: { counts, total: 0 },
        messages: { regionLabel: "Release smoke", title: "Progress", total: String, empty: "No Work" }
      }));
      assert.match(html, /No Work/); break;
    }
    default: throw new Error("Unexpected package " + spec.name);
  }
  if (["@you-agent-factory/components", "@you-agent-factory/factory-visualizers"].includes(spec.name)) {
    const styles = fileURLToPath(import.meta.resolve(spec.name + "/styles.css"));
    assert.ok(realpathSync(styles).startsWith(root + path.sep));
    assert.ok(readFileSync(styles).length > 0, spec.name + " styles must be usable");
  }
  console.log(JSON.stringify({ name: spec.name, version: spec.version, operation: "passed" }));
  } catch (error) { throw new Error(spec.name + "@" + spec.version + " operation failed: " + error.message, { cause: error }); }
}
`;

export async function isolatedConsumerEnvironment(root) {
  // No inherited NODE_PATH, Bun preload or user registry configuration in the consumer.
  const env = {
    ...Object.fromEntries(
      Object.entries(process.env).filter(([key]) =>
        /^(PATH|PATHEXT|SYSTEMROOT|WINDIR|COMSPEC|TEMP|TMP)$/i.test(key),
      ),
    ),
    HOME: path.join(root, "home"),
    USERPROFILE: path.join(root, "home"),
    XDG_CONFIG_HOME: path.join(root, "config"),
    NODE_PATH: "",
    NODE_OPTIONS: "",
    BUN_OPTIONS: "",
    BUN_INSTALL_CACHE_DIR: path.join(root, "cache"),
    npm_config_userconfig: path.join(root, "npmrc"),
  };
  await mkdir(env.HOME);
  await mkdir(env.XDG_CONFIG_HOME);
  await writeFile(
    env.npm_config_userconfig,
    "registry=https://registry.npmjs.org/\n",
  );
  return env;
}

export async function smokeFrontendPackages({
  candidateDirectory,
  evidence,
  runCommand = runSmokeCommand,
  log = (message) => console.error(message),
  expectedSourceCommit,
}) {
  let candidates;
  if (expectedSourceCommit !== undefined) {
    assertSourceCommit(expectedSourceCommit);
    const { validateTaggedReleaseCandidate } = await import(
      "./public-release-package-publish.mjs"
    );
    const validated = await validateTaggedReleaseCandidate({
      candidateDirectory,
      expectedSourceCommit,
    });
    evidence = validated.evidence;
    const root = await realpath(validated.root);
    candidates = await Promise.all(
      evidence.packages.map(async (candidate) => {
        const tarball = await realpath(path.join(root, candidate.tarball));
        if (!tarball.startsWith(root + path.sep))
          throw new Error(`Candidate escapes directory: ${candidate.name}`);
        return { ...candidate, tarball };
      }),
    );
  } else {
    evidence ??= JSON.parse(
      await readFile(
        path.join(candidateDirectory, "public-package-candidates.json"),
        "utf8",
      ),
    );
    candidates = await validateFrontendTarballs(evidence, candidateDirectory);
  }
  const root = await mkdtemp(path.join(tmpdir(), "you-frontend-install-"));
  try {
    const consumer = path.join(root, "consumer");
    await mkdir(consumer);
    const overrides = Object.fromEntries(
      candidates.map(({ name, tarball }) => [
        name,
        tarball.replaceAll("\\", "/"),
      ]),
    );
    const dependencies = {
      ...overrides,
      react: "19.2.0",
      "react-dom": "19.2.0",
    };
    await writeFile(
      path.join(consumer, "package.json"),
      JSON.stringify({
        private: true,
        type: "module",
        dependencies,
        overrides,
      }),
    );
    await writeFile(
      path.join(consumer, "expected.json"),
      JSON.stringify(
        candidates.map(({ name, version }) => ({
          name,
          version,
          sourceCommit: expectedSourceCommit,
        })),
      ),
    );
    await writeFile(path.join(consumer, "smoke.mjs"), frontendConsumerSource);
    const env = await isolatedConsumerEnvironment(root);
    await timePackagePhase(
      "consumer install",
      () =>
        runCommand("bun", ["install", "--ignore-scripts"], {
          cwd: consumer,
          env,
        }),
      { log },
    );
    const result = await timePackagePhase(
      "consumer run",
      () => runCommand("node", ["smoke.mjs"], { cwd: consumer, env }),
      { log },
    );
    log(result.stdout.trim());
    return evidence;
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const { values } = parseArgs({
    options: {
      "candidate-directory": { type: "string" },
      "expected-source-commit": { type: "string" },
    },
    strict: true,
  });
  smokeFrontendPackages({
    candidateDirectory: values["candidate-directory"],
    expectedSourceCommit: values["expected-source-commit"],
  }).then(
    (evidence) => console.log(JSON.stringify(evidence)),
    (error) => {
      console.error(error.message);
      process.exitCode = 1;
    },
  );
}
