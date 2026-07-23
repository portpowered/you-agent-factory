import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import test from "node:test";

import {
	assertReviewedInventory,
	generateManifest,
	generateTypes,
	reviewedPackFiles,
} from "./model-provider-package.mjs";

const root = resolve(import.meta.dirname, "..");

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
	const manifest = JSON.parse(
		await generateManifest(root, async () => `${sourceCommit}\n`),
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
