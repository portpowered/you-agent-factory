import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
	access,
	mkdir,
	mkdtemp,
	readFile,
	realpath,
	rm,
	writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageDirectory = "packages/model-providers";
const typeOutput = `${packageDirectory}/types/index.d.ts`;
const manifestOutput = `${packageDirectory}/metadata/manifest.json`;
const sourceIdentityPaths = Object.freeze([
	"api/openapi.yaml",
	"internal/providercatalog",
	`${packageDirectory}/providers`,
	"scripts/model-provider-package.mjs",
]);
const artifacts = Object.freeze([
	{
		id: "catalog",
		path: "generated/catalog.json",
		title: "Published model-provider catalog",
	},
	{
		id: "provider-catalog-schema",
		path: "generated/provider-catalog.schema.json",
		title: "Published Provider Catalog JSON Schema",
	},
	{
		id: "provider-manifest-schema",
		path: "generated/provider-manifest.schema.json",
		title: "Published Provider Manifest JSON Schema",
	},
]);

export const reviewedPackFiles = Object.freeze([
	"LICENSE.md",
	"README.md",
	...artifacts.map(({ path }) => path),
	"metadata/manifest.json",
	"package.json",
	"types/catalog.d.ts",
	"types/index.d.ts",
	"types/json-schema.d.ts",
	"types/publication-manifest.d.ts",
]);

function normalizedPath(path) {
	return path.replaceAll("\\", "/").replace(/^package\//, "");
}

function sortedUnique(paths) {
	return [...new Set(paths.map(normalizedPath))].sort((left, right) =>
		left.localeCompare(right),
	);
}

function renderPropertyName(name) {
	return /^[A-Za-z_$][A-Za-z0-9_$]*$/.test(name)
		? name
		: JSON.stringify(name);
}

function referenceName(reference) {
	const prefix = "#/$defs/";
	if (!reference.startsWith(prefix)) {
		throw new Error(
			`[model-provider-package] unsupported schema reference ${reference}`,
		);
	}
	return reference.slice(prefix.length);
}

function renderType(schema, indentation = "") {
	if (schema.$ref) {
		return referenceName(schema.$ref);
	}
	if (schema.allOf) {
		if (!Array.isArray(schema.allOf) || schema.allOf.length === 0) {
			throw new Error("[model-provider-package] empty allOf is unsupported");
		}
		return schema.allOf
			.map((entry) => renderType(entry, indentation))
			.join(" & ");
	}
	if (Array.isArray(schema.enum)) {
		return schema.enum.map((value) => JSON.stringify(value)).join(" | ");
	}
	if (schema.type === "array") {
		return `ReadonlyArray<${renderType(schema.items, indentation)}>`;
	}
	if (schema.type === "object") {
		const properties = schema.properties ?? {};
		const required = new Set(schema.required ?? []);
		const entries = Object.entries(properties);
		if (entries.length === 0 && schema.additionalProperties) {
			return `Readonly<Record<string, ${renderType(schema.additionalProperties, indentation)}>>`;
		}
		const nextIndentation = `${indentation}  `;
		const fields = entries.map(([name, property]) => {
			const optional = required.has(name) ? "" : "?";
			return `${nextIndentation}readonly ${renderPropertyName(name)}${optional}: ${renderType(property, nextIndentation)};`;
		});
		return `{\n${fields.join("\n")}\n${indentation}}`;
	}
	if (schema.type === "string") return "string";
	if (schema.type === "boolean") return "boolean";
	if (schema.type === "integer" || schema.type === "number") return "number";
	throw new Error(
		`[model-provider-package] unsupported schema shape ${JSON.stringify(schema)}`,
	);
}

function renderDeclaration(name, schema) {
	const description = schema.description
		? `/** ${schema.description.replaceAll("*/", "* /")} */\n`
		: "";
	return `${description}export type ${name} = ${renderType(schema)};\n`;
}

export function generateTypes(catalogSchema) {
	const declarations = Object.entries(catalogSchema.$defs ?? {})
		.sort(([left], [right]) => left.localeCompare(right))
		.map(([name, schema]) => renderDeclaration(name, schema));
	declarations.push(renderDeclaration("ProviderCatalog", catalogSchema));
	return [
		"// Code generated from provider-catalog.schema.json. DO NOT EDIT.",
		"",
		...declarations,
	].join("\n");
}

function artifactExport({ id, path, title }, payload) {
	const digest = createHash("sha256").update(payload).digest("hex");
	return [
		id,
		{
			path,
			family: "model-providers",
			artifactHash: digest,
			documentation: {
				formatVersion: "1.0.0",
				itemId: id,
				documentation: {
					title: { id: `${id}.title`, canonicalEnglish: title },
					description: {
						id: `${id}.description`,
						canonicalEnglish: `${title} as immutable package data.`,
					},
				},
				examples: [path],
				visibility: "public",
				sourceHash: digest,
			},
			lifecycle: {
				formatVersion: "1.0.0",
				itemId: id,
				state: "active",
				since: "0.0.0",
			},
		},
	];
}

export async function generateManifest(repositoryRoot, runGit = runCommand) {
	const sourceCommit = await resolveSourceCommit(repositoryRoot, runGit);
	const exports = Object.fromEntries(
		await Promise.all(
			artifacts.map(async (artifact) =>
				artifactExport(
					artifact,
					await readFile(join(repositoryRoot, packageDirectory, artifact.path)),
				),
			),
		),
	);
	return `${JSON.stringify(
		{
			formatVersion: "1.0.0",
			packageId: "you-agent-factory.model-providers",
			packageVersion: "0.0.0",
			sourceCommit,
			familyFormatVersions: { "model-providers": "1.0.0" },
			exports,
		},
		null,
		2,
	)}\n`;
}

export async function resolveSourceCommit(repositoryRoot, runGit = runCommand) {
	const git = (...args) => runGit("git", ["-C", repositoryRoot, ...args]);
	const head = (await git("rev-parse", "HEAD")).trim();
	const sourceCommit = (
		await git("rev-list", "-1", "HEAD", "--", ...sourceIdentityPaths)
	).trim();
	if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(sourceCommit)) {
		throw new Error(
			`[model-provider-package] invalid source commit ${JSON.stringify(sourceCommit)}`,
		);
	}
	if (sourceCommit !== head) {
		return sourceCommit;
	}

	const changedPaths = (
		await git(
			"diff-tree",
			"--no-commit-id",
			"--name-only",
			"-r",
			head,
			"--",
			...sourceIdentityPaths,
		)
	).trim();
	if (changedPaths !== "") {
		return sourceCommit;
	}

	let parent;
	try {
		parent = (await git("rev-parse", "HEAD^")).trim();
	} catch {
		if ((await git("rev-parse", "--is-shallow-repository")).trim() === "true") {
			throw shallowSourceHistoryError();
		}
		return sourceCommit;
	}

	let parentSource;
	try {
		parentSource = (
			await git("rev-list", "-1", parent, "--", ...sourceIdentityPaths)
		).trim();
	} catch {
		return sourceCommit;
	}
	if (parentSource !== "" && parentSource !== sourceCommit) {
		try {
			await git("rev-parse", "--verify", `${head}^2`);
			return sourceCommit;
		} catch {
			throw shallowSourceHistoryError();
		}
	}
	return sourceCommit;
}

function shallowSourceHistoryError() {
	return new Error(
		"[model-provider-package] git history is too shallow to determine the last change to package source inputs; fetch full history (for example fetch-depth: 0 in CI)",
	);
}

function runCommand(command, arguments_, options = {}) {
	return new Promise((resolvePromise, rejectPromise) => {
		const child = spawn(command, arguments_, {
			...options,
			shell: process.platform === "win32" && command === "npm",
			stdio: ["ignore", "pipe", "pipe"],
		});
		let stdout = "";
		let stderr = "";
		child.stdout.setEncoding("utf8");
		child.stderr.setEncoding("utf8");
		child.stdout.on("data", (chunk) => {
			stdout += chunk;
		});
		child.stderr.on("data", (chunk) => {
			stderr += chunk;
		});
		child.on("error", rejectPromise);
		child.on("close", (status) => {
			if (status === 0) {
				resolvePromise(stdout);
				return;
			}
			rejectPromise(
				new Error(
					`[model-provider-package] ${command} exited ${status}\n${[stdout, stderr].filter(Boolean).join("\n").trim()}`,
				),
			);
		});
	});
}

export function assertReviewedInventory(actualFiles) {
	const actual = sortedUnique(actualFiles);
	const expected = sortedUnique(reviewedPackFiles);
	const actualSet = new Set(actual);
	const expectedSet = new Set(expected);
	const unexpected = actual.filter((path) => !expectedSet.has(path));
	const missing = expected.filter((path) => !actualSet.has(path));
	if (unexpected.length === 0 && missing.length === 0) return;
	throw new Error(
		[
			"[model-provider-package] tarball inventory rejected",
			...unexpected.map((path) => `unexpected: ${path}`),
			...missing.map((path) => `missing: ${path}`),
		].join("\n"),
	);
}

export async function generate(repositoryRoot) {
	const schema = JSON.parse(
		await readFile(
			join(
				repositoryRoot,
				packageDirectory,
				"generated/provider-catalog.schema.json",
			),
			"utf8",
		),
	);
	const generatedTypes = generateTypes(schema);
	const generatedManifest = await generateManifest(repositoryRoot);
	await mkdir(dirname(join(repositoryRoot, typeOutput)), { recursive: true });
	await mkdir(dirname(join(repositoryRoot, manifestOutput)), { recursive: true });
	await writeFile(
		join(repositoryRoot, typeOutput),
		generatedTypes,
		"utf8",
	);
	await writeFile(
		join(repositoryRoot, manifestOutput),
		generatedManifest,
		"utf8",
	);
}

export async function check(repositoryRoot) {
	const schema = JSON.parse(
		await readFile(
			join(
				repositoryRoot,
				packageDirectory,
				"generated/provider-catalog.schema.json",
			),
			"utf8",
		),
	);
	const expected = new Map([
		[typeOutput, generateTypes(schema)],
		[manifestOutput, await generateManifest(repositoryRoot)],
	]);
	for (const [path, contents] of expected) {
		const actual = await readFile(join(repositoryRoot, path), "utf8").catch(
			() => undefined,
		);
		if (actual !== contents) {
			throw new Error(
				`[model-provider-package] stale ${path}; run make model-provider-package-generate`,
			);
		}
	}
}

export async function packAndVerify(repositoryRoot, destination) {
	const stdout = await runCommand("npm", [
		"pack",
		"--json",
		"--ignore-scripts",
		"--pack-destination",
		destination,
		join(repositoryRoot, packageDirectory),
	]);
	const reports = JSON.parse(stdout);
	if (!Array.isArray(reports) || reports.length !== 1) {
		throw new Error("[model-provider-package] npm pack returned invalid output");
	}
	const report = reports[0];
	assertReviewedInventory(report.files.map(({ path }) => path));
	const tarballPath = join(destination, report.filename);
	await access(tarballPath);
	return { files: sortedUnique(report.files.map(({ path }) => path)), tarballPath };
}

export async function verifyCleanConsumer(repositoryRoot, tarballPath) {
	const consumerRoot = await mkdtemp(join(tmpdir(), "you-model-providers-"));
	try {
		await writeFile(
			join(consumerRoot, "package.json"),
			'{"private":true,"type":"module"}\n',
		);
		await runCommand(
			"npm",
			[
				"install",
				"--ignore-scripts",
				"--no-audit",
				"--no-fund",
				tarballPath,
			],
			{ cwd: consumerRoot },
		);
		const consumerSource = [
			'import catalog from "@you-agent-factory/model-providers/catalog" with { type: "json" };',
			'import manifestSchema from "@you-agent-factory/model-providers/schemas/provider-manifest" with { type: "json" };',
			'import catalogSchema from "@you-agent-factory/model-providers/schemas/provider-catalog" with { type: "json" };',
			'import publication from "@you-agent-factory/model-providers/manifest" with { type: "json" };',
			'import type { ProviderCatalog, ProviderManifest } from "@you-agent-factory/model-providers/types";',
			"const typedCatalog: ProviderCatalog = catalog;",
			"const provider: ProviderManifest = typedCatalog.providers[0];",
			"void [provider, manifestSchema, catalogSchema, publication];",
		].join("\n");
		await writeFile(join(consumerRoot, "consumer.ts"), consumerSource);
		await writeFile(
			join(consumerRoot, "tsconfig.json"),
			JSON.stringify({
				compilerOptions: {
					module: "NodeNext",
					moduleResolution: "NodeNext",
					esModuleInterop: true,
					noEmit: true,
					strict: true,
					target: "ES2022",
				},
				files: ["consumer.ts"],
			}),
		);
		const compiler = join(
			repositoryRoot,
			"ui/node_modules/typescript/bin/tsc",
		);
		await access(compiler);
		await runCommand(process.execPath, [compiler, "-p", "tsconfig.json"], {
			cwd: consumerRoot,
		});

		const runtimeExports = [
			{
				id: "catalog",
				specifier: "@you-agent-factory/model-providers/catalog",
				path: "generated/catalog.json",
			},
			{
				id: "provider-manifest-schema",
				specifier:
					"@you-agent-factory/model-providers/schemas/provider-manifest",
				path: "generated/provider-manifest.schema.json",
			},
			{
				id: "provider-catalog-schema",
				specifier:
					"@you-agent-factory/model-providers/schemas/provider-catalog",
				path: "generated/provider-catalog.schema.json",
			},
			{
				id: "manifest",
				specifier: "@you-agent-factory/model-providers/manifest",
				path: "metadata/manifest.json",
			},
		];
		const expectedRuntimeHashes = Object.fromEntries(
			await Promise.all(
				runtimeExports.map(async ({ id, path }) => [
					id,
					createHash("sha256")
						.update(
							await readFile(join(repositoryRoot, packageDirectory, path)),
						)
						.digest("hex"),
				]),
			),
		);
		const runtimeSource = [
			'import assert from "node:assert/strict";',
			'import { createHash } from "node:crypto";',
			'import { readFile } from "node:fs/promises";',
			'import catalog from "@you-agent-factory/model-providers/catalog" with { type: "json" };',
			'import manifestSchema from "@you-agent-factory/model-providers/schemas/provider-manifest" with { type: "json" };',
			'import catalogSchema from "@you-agent-factory/model-providers/schemas/provider-catalog" with { type: "json" };',
			'import publication from "@you-agent-factory/model-providers/manifest" with { type: "json" };',
			`const expectedHashes = ${JSON.stringify(expectedRuntimeHashes)};`,
			`const runtimeExports = ${JSON.stringify(runtimeExports)};`,
			'assert.equal(catalog.providers[0].id, "agy");',
			'assert.equal(manifestSchema.$id, "https://schemas.you.dev/model-providers/provider-manifest/1.0.0.schema.json");',
			'assert.equal(catalogSchema.$id, "https://schemas.you.dev/model-providers/provider-catalog/1.0.0.schema.json");',
			'assert.equal(publication.packageId, "you-agent-factory.model-providers");',
			"for (const { id, specifier } of runtimeExports) {",
			"  const payload = await readFile(new URL(import.meta.resolve(specifier)));",
			'  assert.equal(createHash("sha256").update(payload).digest("hex"), expectedHashes[id], `${specifier} bytes`);',
			"}",
		].join("\n");
		await writeFile(join(consumerRoot, "consumer.mjs"), runtimeSource);
		await runCommand(process.execPath, ["consumer.mjs"], {
			cwd: consumerRoot,
		});

		const installedRoot = await realpath(
			join(
				consumerRoot,
				"node_modules/@you-agent-factory/model-providers",
			),
		);
		for (const { path } of artifacts) {
			const canonical = await readFile(
				join(repositoryRoot, packageDirectory, path),
			);
			const installed = await readFile(join(installedRoot, path));
			if (!canonical.equals(installed)) {
				throw new Error(
					`[model-provider-package] installed artifact differs: ${path}`,
				);
			}
		}
		const installedManifest = JSON.parse(
			await readFile(join(installedRoot, "metadata/manifest.json"), "utf8"),
		);
		for (const [id, entry] of Object.entries(installedManifest.exports)) {
			const payload = await readFile(join(installedRoot, entry.path));
			const digest = createHash("sha256").update(payload).digest("hex");
			if (digest !== entry.artifactHash) {
				throw new Error(
					`[model-provider-package] provenance hash mismatch: ${id}`,
				);
			}
		}
	} finally {
		await rm(consumerRoot, { recursive: true, force: true });
	}
}

async function main() {
	const repositoryRoot = resolve(
		dirname(fileURLToPath(import.meta.url)),
		"..",
	);
	const command = process.argv[2];
	if (command === "generate") {
		await generate(repositoryRoot);
		return;
	}
	if (command === "check") {
		await check(repositoryRoot);
		return;
	}
	if (command === "smoke") {
		await check(repositoryRoot);
		const destination = await mkdtemp(join(tmpdir(), "you-model-pack-"));
		try {
			const packed = await packAndVerify(repositoryRoot, destination);
			await verifyCleanConsumer(repositoryRoot, packed.tarballPath);
		} finally {
			await rm(destination, { recursive: true, force: true });
		}
		return;
	}
	throw new Error(
		"usage: node scripts/model-provider-package.mjs <generate|check|smoke>",
	);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	main().catch((error) => {
		console.error(error.message);
		process.exitCode = 1;
	});
}
