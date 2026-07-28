import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
	concreteExportCases,
	installAndVerifyTarball,
	verifyResolvedExport,
} from "./api-package-consumer.mjs";
import { packAndVerify } from "./api-package-pack.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const packageDirectory = join(repositoryRoot, "packages", "api");
const protectedFiles = [
	"api/openapi-main.yaml",
	"api/openapi.yaml",
	"contracts/cli/commands.json",
	"contracts/cli/command-manifest.schema.json",
	"contracts/testdata/baseline/cli-commands.json",
	"contracts/testdata/baseline/mcp-tools.json",
	"contracts/common/deprecations.schema.json",
	"contracts/common/documentation.schema.json",
	"contracts/manifest.schema.json",
	"contracts/config/you-config.schema.json",
	"contracts/config/mock-workers.schema.json",
	"packages/api/generated/joined/contracts/common/deprecations.schema.json",
	"packages/api/generated/joined/contracts/common/documentation.schema.json",
	"packages/api/generated/joined/contracts/manifest.schema.json",
	"packages/api/generated/manifest.json",
	"packages/api/generated/openapi/openapi.yaml",
	"packages/api/generated/cli/commands.json",
	"packages/api/generated/cli/command-manifest.schema.json",
	"packages/api/generated/mcp/tools.json",
	"packages/api/generated/schemas/you-config.schema.json",
	"packages/api/generated/schemas/factory.schema.json",
	"packages/api/generated/schemas/factory-event.schema.json",
	"packages/api/generated/schemas/factory-recording.schema.json",
	"packages/api/generated/schemas/mock-workers.schema.json",
	"packages/api/generated/javascript/runtime-api.json",
	"pkg/services/workers/internal/interface/testdata/baseline/mock-workers-topology.json",
	"pkg/services/factory_runtime/tooling/javascript/javascript-runtime-symbols.json",
	"pkg/transports/http/generated/server.gen.go",
	"pkg/transports/http/client/client.gen.go",
	"ui/src/api/generated/openapi.ts",
];

async function temporaryDirectory(t, name) {
	const directory = await mkdtemp(join(tmpdir(), name));
	t.after(() => rm(directory, { recursive: true, force: true }));
	return directory;
}

async function hashes(paths) {
	return Object.fromEntries(
		await Promise.all(
			paths.map(async (path) => [
				path,
				createHash("sha256")
					.update(await readFile(join(repositoryRoot, ...path.split("/"))))
					.digest("hex"),
			]),
		),
	);
}

test("real tarball resolves every currently staged export from an isolated install", async (t) => {
	const before = await hashes(protectedFiles);
	const packDestination = await temporaryDirectory(t, "you-api-consumer-pack-");
	const consumerDirectory = await temporaryDirectory(t, "you-api-consumer-");
	await writeFile(
		join(consumerDirectory, "package.json"),
		'{"name":"api-tarball-consumer","private":true}\n',
	);

	const packed = await packAndVerify({ packageDirectory, packDestination });
	const exportCases = await installAndVerifyTarball({
		consumerDirectory,
		packageName: packed.packageName,
		packedFiles: packed.files,
		tarballPath: packed.tarballPath,
		workspaceDirectory: repositoryRoot,
	});

	assert.deepEqual(
		exportCases.map(({ specifier }) => specifier),
		[
			"@you-agent-factory/api/cli",
			"@you-agent-factory/api/javascript/runtime",
			"@you-agent-factory/api/joined/contracts/common/deprecations.schema.json",
			"@you-agent-factory/api/joined/contracts/common/documentation.schema.json",
			"@you-agent-factory/api/joined/contracts/manifest.schema.json",
			"@you-agent-factory/api/manifest",
			"@you-agent-factory/api/mcp",
			"@you-agent-factory/api/openapi",
			"@you-agent-factory/api/schemas/cli-command-manifest",
			"@you-agent-factory/api/schemas/factory",
			"@you-agent-factory/api/schemas/factory-event",
			"@you-agent-factory/api/schemas/factory-recording",
			"@you-agent-factory/api/schemas/mock-workers",
			"@you-agent-factory/api/schemas/you-config",
		],
	);
	assert.deepEqual(await hashes(protectedFiles), before);
});

test("concrete exports include named artifacts and every wildcard match", () => {
	assert.deepEqual(
		concreteExportCases(
			{
				name: "example-package",
				exports: {
					"./manifest": "./generated/manifest.json",
					"./joined/*": "./generated/joined/*",
				},
			},
			[
				"package.json",
				"generated/manifest.json",
				"generated/joined/one.json",
				"generated/joined/nested/two.yaml",
			],
		),
		[
			{
				specifier: "example-package/joined/nested/two.yaml",
				target: "generated/joined/nested/two.yaml",
			},
			{
				specifier: "example-package/joined/one.json",
				target: "generated/joined/one.json",
			},
			{
				specifier: "example-package/manifest",
				target: "generated/manifest.json",
			},
		],
	);
});

test("concrete exports reject every missing named target", () => {
	assert.throws(
		() =>
			concreteExportCases(
				{
					name: "example-package",
					exports: {
						"./openapi": "./generated/openapi.yaml",
						"./manifest": "./generated/manifest.json",
						"./joined/*": "./generated/joined/*",
					},
				},
				["generated/joined/one.json"],
			),
		{
			message: [
				"[api-package-consumer] missing named export targets:",
				"  ./manifest -> generated/manifest.json",
				"  ./openapi -> generated/openapi.yaml",
			].join("\n"),
		},
	);
});

test("consumer verification rejects missing, escaped, and invalid raw targets", async (t) => {
	const fixture = await temporaryDirectory(t, "you-api-consumer-invalid-");
	const packageRoot = join(fixture, "package");
	const validTarget = join(packageRoot, "generated", "valid.json");
	const invalidTarget = join(packageRoot, "generated", "invalid.json");
	const outsideTarget = join(fixture, "outside.json");
	await mkdir(dirname(validTarget), { recursive: true });
	await writeFile(validTarget, "{}\n");
	await writeFile(invalidTarget, "not json\n");
	await writeFile(outsideTarget, "{}\n");

	const base = { packageRoot, target: "generated/valid.json" };
	await assert.rejects(
		verifyResolvedExport({
			...base,
			specifier: "example-package/missing",
			resolveSpecifier: async () =>
				join(packageRoot, "generated", "missing.json"),
		}),
		/export target is missing: example-package\/missing/,
	);
	await assert.rejects(
		verifyResolvedExport({
			...base,
			specifier: "example-package/escaped",
			resolveSpecifier: async () => outsideTarget,
		}),
		/export resolved outside installed package: example-package\/escaped/,
	);
	await assert.rejects(
		verifyResolvedExport({
			...base,
			specifier: "example-package/invalid",
			target: "generated/invalid.json",
			resolveSpecifier: async () => invalidTarget,
		}),
		/export is not valid JSON: example-package\/invalid/,
	);
});

test("consumer verification rejects semantically substituted manifest and schema artifacts", async (t) => {
	const fixture = await temporaryDirectory(t, "you-api-consumer-semantics-");
	const packageRoot = join(fixture, "package");
	const substitutions = [
		["manifest", { name: "@you-agent-factory/api", version: "0.0.0" }],
		["schemas/you-config", { formatVersion: "global-config-topology/v1" }],
		["schemas/factory", { formatVersion: "factory-openapi-parity/v1" }],
		["schemas/mock-workers", { formatVersion: "mock-workers-topology/v1" }],
	];
	for (const [specifierSuffix, document] of substitutions) {
		const target = join(
			packageRoot,
			...specifierSuffix.split("/"),
			"artifact.json",
		);
		await mkdir(dirname(target), { recursive: true });
		await writeFile(target, `${JSON.stringify(document)}\n`);
		await assert.rejects(
			verifyResolvedExport({
				packageRoot,
				resolveSpecifier: async () => target,
				specifier: `@you-agent-factory/api/${specifierSuffix}`,
				target: normalizedFixtureTarget(packageRoot, target),
			}),
			/contract manifest shape|configuration JSON Schema semantics/,
		);
	}
});

function normalizedFixtureTarget(packageRoot, target) {
	return target.slice(packageRoot.length + 1).replaceAll("\\", "/");
}
