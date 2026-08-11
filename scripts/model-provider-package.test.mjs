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
	assert.match(declarations, /readonly defaultEnabled\?: boolean \| null;/);
	assert.match(declarations, /export type ProviderModality =[^]*readonly condition: string;/);
	assert.match(declarations, /export type ProviderTool =[^]*readonly condition: string;/);
});

test("generated schema and declarations preserve expanded capability vocabulary", async () => {
	const schema = JSON.parse(
		await readFile(
			resolve(
				root,
				"packages/model-providers/generated/provider-catalog.schema.json",
			),
			"utf8",
		),
	);
	const definitions = schema.$defs;
	for (const [name, values] of Object.entries({
		ProviderACPResourceDelivery: ["implemented", "unsupported", "conditional", "unknown"],
		ProviderCapabilityEvidenceKind: ["primary_documentation", "protocol_probe", "conformance_fixture", "maintainer_assertion"],
		ProviderCapabilitySupport: ["supported", "unsupported", "conditional", "unknown"],
		ProviderHarnessKind: ["native_cli", "acp"],
		ProviderModelCatalogPosture: ["exact", "runtime_discovered", "operator_selected", "unknown"],
		ProviderModalityDirection: ["input", "output"],
		ProviderModalityKind: ["text", "image", "audio", "video"],
		ProviderModalitySupport: ["supported", "unsupported", "conditional", "unknown"],
		ProviderModalityTransport: ["inline", "file_path", "acp_resource", "tool_mediated", "none"],
		ProviderToolAvailability: ["built_in", "optional", "operator_configured", "external", "unknown"],
		ProviderToolSupport: ["supported", "unsupported", "conditional", "unknown"],
	})) {
		assert.deepEqual(definitions[name].enum, values, `${name} enum`);
	}
	assert.deepEqual(definitions.ProviderManifest.required, [
		"id", "aliases", "displayName", "description", "documentation",
		"technicalSupportLevel", "implementationAvailability",
		"maximumExecutionCapabilities", "maximumResponseFidelityCapabilities", "discovery",
	]);
	assert.deepEqual(definitions.ProviderCapabilityEvidence.required, ["id", "kind", "verifiedOn"]);
	assert.deepEqual(definitions.ProviderModality.required, ["direction", "modality", "support", "transport"]);
	assert.deepEqual(definitions.ProviderTool.required, ["name", "support", "description"]);
	assert.deepEqual(definitions.ProviderToolOutputModality.required, ["modality", "support", "transport"]);
	assert.deepEqual(definitions.ProviderTool.properties.defaultEnabled.type, ["boolean", "null"]);
	assert.equal(
		definitions.ProviderTool.properties.outputModalities.items.$ref,
		"#/$defs/ProviderToolOutputModality",
	);
	assert.equal(
		definitions.ProviderModality.properties.evidenceRefs.items.pattern,
		"^[a-z][a-z0-9._-]*$",
	);

	const declarations = generateTypes(schema);
	for (const value of [
		'"conditional"',
		'"unknown"',
		'"acp_resource"',
		'"tool_mediated"',
		'"operator_configured"',
	]) {
		assert.ok(declarations.includes(value), `${value} is missing from declarations`);
	}
	assert.match(declarations, /readonly defaultEnabled\?: boolean \| null;/);
	assert.match(declarations, /readonly evidenceRefs\?: ReadonlyArray<string>;/);
});

test("development manifest is commit-independent and records exact artifact hashes", async () => {
	const manifest = JSON.parse(await generateManifest(root));
	assert.equal(manifest.sourceCommit, undefined);
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
