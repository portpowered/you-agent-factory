import { spawn } from "node:child_process";
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

export function runSmokeCommand(
  command,
  args,
  { cwd, env = process.env, timeoutMs = 120_000 } = {},
) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd,
      env,
      shell: false,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    const timer = setTimeout(() => {
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
      if (code === 0) resolve({ stdout, stderr });
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
  assert.ok(realpathSync(fileURLToPath(import.meta.resolve(spec.name))).startsWith(root + path.sep));
  const api = await import(spec.name);
  switch (spec.name) {
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

export async function smokeFrontendPackages({
  candidateDirectory,
  evidence,
  runCommand = runSmokeCommand,
  log = (message) => console.error(message),
}) {
  evidence ??= JSON.parse(
    await readFile(
      path.join(candidateDirectory, "public-package-candidates.json"),
      "utf8",
    ),
  );
  const candidates = await validateFrontendTarballs(
    evidence,
    candidateDirectory,
  );
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
        candidates.map(({ name, version }) => ({ name, version })),
      ),
    );
    await writeFile(path.join(consumer, "smoke.mjs"), frontendConsumerSource);
    // No inherited NODE_PATH, Bun preload or user registry configuration in the consumer.
    const env = {
      ...process.env,
      NODE_PATH: "",
      NODE_OPTIONS: "",
      BUN_OPTIONS: "",
      BUN_INSTALL_CACHE_DIR: path.join(root, "cache"),
      npm_config_userconfig: path.join(root, "npmrc"),
    };
    await writeFile(
      env.npm_config_userconfig,
      "registry=https://registry.npmjs.org/\n",
    );
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
    options: { "candidate-directory": { type: "string" } },
    strict: true,
  });
  smokeFrontendPackages({
    candidateDirectory: values["candidate-directory"],
  }).then(
    (evidence) => console.log(JSON.stringify(evidence)),
    (error) => {
      console.error(error.message);
      process.exitCode = 1;
    },
  );
}
