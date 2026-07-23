import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { prepareCandidate } from "./packaged-factories-package-candidate.mjs";
import {
  installAndVerifyTarball,
  npmInstallArguments,
  runNpmInstall,
} from "./packaged-factories-package-consumer.mjs";
import {
  copyableInvocation,
  resolvePublicArtifact,
  selectLocalizedValue,
  verifyFactory,
} from "./packaged-factories-package-consumer-contract.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const packageDirectory = join(repositoryRoot, "packages", "packaged-factories");
const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

async function temporaryDirectory(t, name) {
  const directory = await mkdtemp(join(tmpdir(), name));
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}

function digest(contents) {
  return createHash("sha256").update(contents).digest("hex");
}

test("exact candidate installs outside the workspace and verifies every public Factory export", {
  timeout: 120_000,
}, async (t) => {
  const outputDirectory = await temporaryDirectory(
    t,
    "you-packaged-factories-candidate-",
  );
  const candidate = await prepareCandidate({
    packageDirectory,
    outputDirectory,
    runId: "42",
    sourceCommit,
    verifyGeneratedCatalog: async () => {},
  });
  let consumerDirectory;
  const evidence = await installAndVerifyTarball({
    expectedSourceCommit: sourceCommit,
    expectedVersion: candidate.evidence.candidateVersion,
    npmInstall: async (directory, tarballPath) => {
      consumerDirectory = directory;
      await runNpmInstall(directory, tarballPath);
    },
    tarballPath: candidate.tarballPath,
    workspaceDirectory: repositoryRoot,
  });

  assert.equal(evidence.packageName, candidate.evidence.packageName);
  assert.equal(evidence.packageVersion, candidate.evidence.candidateVersion);
  assert.equal(evidence.sourceCommit, sourceCommit);
  assert.equal(evidence.factoryCount, 7);
  assert.equal(evidence.verifiedSpecifiers.length, 17);
  assert.match(evidence.localizedDescription.value, /\S/);
  assert.ok(Object.keys(evidence.copyableInvocation.args).length > 0);
  await assert.rejects(access(consumerDirectory), { code: "ENOENT" });
});

test("npm install disables lifecycle, lockfile, workspace, and link behavior", () => {
  const arguments_ = npmInstallArguments("candidate.tgz");
  for (const flag of [
    "--ignore-scripts",
    "--no-audit",
    "--no-fund",
    "--no-save",
    "--package-lock=false",
    "--workspaces=false",
    "--install-links=false",
  ]) {
    assert.ok(arguments_.includes(flag), `missing ${flag}`);
  }
  assert.ok(arguments_.includes("ajv@8.20.0"));
  assert.ok(arguments_.includes("ajv-formats@3.0.1"));
  assert.ok(arguments_.includes("yaml@2.9.0"));
});

test("localized descriptions use exact locale then base fallback", () => {
  const asset = {
    type: "LOCALIZABLE_ASSET",
    value: "Fallback description",
    values: { "fr-CA": "Description exacte" },
  };
  assert.equal(selectLocalizedValue(asset, "fr-CA"), "Description exacte");
  assert.equal(selectLocalizedValue(asset, "fr"), "Fallback description");
  assert.throws(
    () =>
      selectLocalizedValue(
        { type: "LOCALIZABLE_ASSET", value: "", values: {} },
        "en-US",
      ),
    /no usable exact-locale or fallback value/,
  );
});

test("copyable invocation selection requires non-empty installed args", () => {
  assert.deepEqual(
    copyableInvocation({
      factories: [
        { name: "@you/empty", examples: [{ args: {} }] },
        {
          name: "@you/review",
          examples: [{ args: { input: "Review this" } }],
        },
      ],
    }),
    { factory: "@you/review", args: { input: "Review this" } },
  );
  assert.throws(
    () => copyableInvocation({ factories: [{ examples: [{ args: {} }] }] }),
    /no copyable invocation payload/,
  );
});

test("public export verification rejects unresolved and package-external targets", async (t) => {
  const fixture = await temporaryDirectory(t, "you-consumer-exports-");
  const packageRoot = join(fixture, "package");
  const outside = join(fixture, "outside.json");
  await mkdir(packageRoot);
  await writeFile(outside, "{}\n");
  await assert.rejects(
    resolvePublicArtifact({
      expectedPath: "generated/manifest.json",
      packageRoot,
      resolveSpecifier: async () => {
        throw new Error("missing");
      },
      specifier: "example/manifest",
    }),
    /public export did not resolve: example\/manifest/,
  );
  await assert.rejects(
    resolvePublicArtifact({
      expectedPath: "generated/manifest.json",
      packageRoot,
      resolveSpecifier: async () => outside,
      specifier: "example/manifest",
    }),
    /public export resolved outside installed package: example\/manifest/,
  );
});

test("Factory verification rejects hash, representation, and schema failures", async (t) => {
  const packageRoot = await temporaryDirectory(t, "you-consumer-factory-");
  const jsonPath = join(
    packageRoot,
    "generated",
    "factories",
    "demo",
    "factory.json",
  );
  const yamlPath = join(
    packageRoot,
    "generated",
    "factories",
    "demo",
    "factory.yaml",
  );
  const jsonContents = Buffer.from('{"name":"demo"}\n');
  const yamlContents = Buffer.from('{"name":"demo"}\n');
  await mkdir(dirname(jsonPath), { recursive: true });
  await writeFile(jsonPath, jsonContents);
  await writeFile(yamlPath, yamlContents);
  const entry = {
    slug: "demo",
    json: {
      locator: "generated/factories/demo/factory.json",
      sha256: digest(jsonContents),
    },
    yaml: {
      locator: "generated/factories/demo/factory.yaml",
      sha256: digest(yamlContents),
    },
  };
  const resolveSpecifier = async (specifier) =>
    specifier.endsWith(".json") ? jsonPath : yamlPath;
  const input = {
    entry,
    packageName: "example",
    packageRoot,
    parseYAML: JSON.parse,
    resolveSpecifier,
    validate: () => true,
  };
  assert.deepEqual(await verifyFactory(input), [
    "example/factories/demo.json",
    "example/factories/demo.yaml",
  ]);

  await assert.rejects(
    verifyFactory({
      ...input,
      entry: { ...entry, json: { ...entry.json, sha256: "0".repeat(64) } },
    }),
    /manifest hash mismatch: example\/factories\/demo.json/,
  );
  await writeFile(yamlPath, '{"name":"different"}\n');
  const divergent = await readFile(yamlPath);
  await assert.rejects(
    verifyFactory({
      ...input,
      entry: {
        ...entry,
        yaml: { ...entry.yaml, sha256: digest(divergent) },
      },
    }),
    /JSON\/YAML Factory representations diverge: demo/,
  );
  await writeFile(yamlPath, yamlContents);
  await assert.rejects(
    verifyFactory({
      ...input,
      validate: Object.assign(() => false, {
        errors: [{ message: "invalid fixture" }],
      }),
    }),
    /Factory is invalid against installed schema/,
  );
});
