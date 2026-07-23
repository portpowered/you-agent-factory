import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import {
	mkdir,
	mkdtemp,
	readFile,
	rm,
	writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { promisify } from "node:util";

import {
	assertReviewedInventory,
	generateManifest,
	generateTypes,
	resolveSourceCommit,
	reviewedPackFiles,
} from "./model-provider-package.mjs";

const root = resolve(import.meta.dirname, "..");
const executeFile = promisify(execFile);

async function git(repositoryRoot, ...args) {
	const { stdout } = await executeFile("git", ["-C", repositoryRoot, ...args]);
	return stdout;
}

async function assertSourceRevisionAdvances(sourcePath) {
	const repositoryRoot = await mkdtemp(
		join(tmpdir(), "you-model-provider-source-git-"),
	);
	try {
		await mkdir(
			join(repositoryRoot, "packages/model-providers/providers/agy"),
			{ recursive: true },
		);
		const changedPath = join(repositoryRoot, sourcePath);
		await mkdir(dirname(changedPath), { recursive: true });
		await writeFile(
			join(
				repositoryRoot,
				"packages/model-providers/providers/agy/provider.yaml",
			),
			"id: agy\n",
		);
		await writeFile(changedPath, "// initial source\n");
		await git(repositoryRoot, "init");
		await git(repositoryRoot, "config", "user.name", "Provider Catalog Test");
		await git(
			repositoryRoot,
			"config",
			"user.email",
			"provider-catalog@example.com",
		);
		await git(repositoryRoot, "add", "-A");
		await git(repositoryRoot, "commit", "-m", "initial package source");

		await writeFile(changedPath, "// revised source\n");
		await git(repositoryRoot, "add", "-A");
		await git(repositoryRoot, "commit", "-m", "revise package source");

		const head = (await git(repositoryRoot, "rev-parse", "HEAD")).trim();
		assert.equal(await resolveSourceCommit(repositoryRoot), head);
	} finally {
		await rm(repositoryRoot, { recursive: true, force: true });
	}
}

test("generated declarations come from the explicit Provider Catalog schema", async () => {
	const schema = JSON.parse(
		await readFile(
			resolve(
				root,
				"packages/model-providers/generated/provider-catalog.schema.json",
			),
			"utf8",
		),
	);
	const declarations = generateTypes(schema);
	assert.match(declarations, /export type ProviderCatalog =/);
	assert.match(declarations, /readonly providers: ReadonlyArray<ProviderManifest>/);
	assert.match(
		declarations,
		/export type ProviderTechnicalSupportLevel = "production" \| "experimental" \| "not-supported"/,
	);
});

test("publication manifest records immutable source and exact artifact hashes", async () => {
	const sourceCommit = "a".repeat(40);
	const headCommit = "b".repeat(40);
	const manifest = JSON.parse(
		await generateManifest(root, async (_, args) =>
			args.includes("rev-list") ? `${sourceCommit}\n` : `${headCommit}\n`,
		),
	);
	assert.equal(manifest.sourceCommit, sourceCommit);
	assert.equal(manifest.packageId, "you-agent-factory.model-providers");
	for (const entry of Object.values(manifest.exports)) {
		const payload = await readFile(
			resolve(root, "packages/model-providers", entry.path),
		);
		assert.equal(
			entry.artifactHash,
			createHash("sha256").update(payload).digest("hex"),
		);
	}
});

test("source revision rejects an unrelated shallow-clone head", async () => {
	const fixtureRoot = await mkdtemp(join(tmpdir(), "you-model-provider-git-"));
	const origin = join(fixtureRoot, "origin");
	const shallow = join(fixtureRoot, "shallow");
	try {
		await mkdir(join(origin, "packages/model-providers/providers/agy"), {
			recursive: true,
		});
		await writeFile(
			join(origin, "packages/model-providers/providers/agy/provider.yaml"),
			"id: agy\n",
		);
		await git(origin, "init");
		await git(origin, "config", "user.name", "Provider Catalog Test");
		await git(origin, "config", "user.email", "provider-catalog@example.com");
		await git(origin, "add", "-A");
		await git(origin, "commit", "-m", "provider source");
		await writeFile(join(origin, "unrelated.txt"), "follow-up\n");
		await git(origin, "add", "-A");
		await git(origin, "commit", "-m", "unrelated follow-up");

		await executeFile("git", [
			"clone",
			"--depth",
			"1",
			pathToFileURL(origin).href,
			shallow,
		]);

		await assert.rejects(
			resolveSourceCommit(shallow),
			/too shallow.*fetch full history/i,
		);
	} finally {
		await rm(fixtureRoot, { recursive: true, force: true });
	}
});

test("source revision advances when the package generator changes", async () => {
	await assertSourceRevisionAdvances("scripts/model-provider-package.mjs");
});

test("source revision advances when the schema converter changes", async () => {
	await assertSourceRevisionAdvances(
		"internal/contractopenapiconverter/convert.go",
	);
});

test("reviewed tarball inventory rejects provider sources and runtime code", () => {
	assert.doesNotThrow(() => assertReviewedInventory(reviewedPackFiles));
	assert.throws(
		() =>
			assertReviewedInventory([
				...reviewedPackFiles,
				"providers/codex/provider.yaml",
			]),
		/unexpected: providers\/codex\/provider.yaml/,
	);
	assert.throws(
		() =>
			assertReviewedInventory(
				reviewedPackFiles.filter((path) => path !== "generated/catalog.json"),
			),
		/missing: generated\/catalog.json/,
	);
});

test("package metadata has no executable or dependency surface", async () => {
	const packageManifest = JSON.parse(
		await readFile(
			resolve(root, "packages/model-providers/package.json"),
			"utf8",
		),
	);
	for (const field of [
		"scripts",
		"bin",
		"main",
		"dependencies",
		"optionalDependencies",
		"peerDependencies",
	]) {
		assert.equal(packageManifest[field], undefined, `${field} must be absent`);
	}
	assert.deepEqual(
		Object.keys(packageManifest.exports).sort(),
		[
			"./catalog",
			"./manifest",
			"./schemas/provider-catalog",
			"./schemas/provider-manifest",
			"./types",
		],
	);
});
