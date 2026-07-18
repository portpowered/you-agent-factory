import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { REVIEWED_PACK_FILES } from "./api-package-pack.mjs";

const packageManifest = JSON.parse(
	await readFile(
		new URL("../packages/api/package.json", import.meta.url),
		"utf8",
	),
);

const expectedExports = {
	"./manifest": "./generated/manifest.json",
	"./openapi": "./generated/openapi/openapi.yaml",
	"./cli": "./generated/cli/commands.json",
	"./mcp": "./generated/mcp/tools.json",
	"./schemas/you-config": "./generated/schemas/you-config.schema.json",
	"./schemas/factory": "./generated/schemas/factory.schema.json",
	"./schemas/factory-event": "./generated/schemas/factory-event.schema.json",
	"./schemas/factory-recording":
		"./generated/schemas/factory-recording.schema.json",
	"./schemas/mock-workers": "./generated/schemas/mock-workers.schema.json",
	"./javascript/runtime": "./generated/javascript/runtime-api.json",
	"./joined/*": "./generated/joined/*",
};

test("raw artifact package exposes the exact supported subpaths", () => {
	assert.equal(packageManifest.name, "@you-agent-factory/api");
	assert.deepEqual(packageManifest.exports, expectedExports);
	assert.equal(
		Object.hasOwn(packageManifest.exports, "./components/*"),
		false,
		"package.json must not declare ./components/* exports",
	);
});

test("raw artifact package has no executable or runtime dependency surface", () => {
	for (const field of [
		"main",
		"module",
		"browser",
		"bin",
		"scripts",
		"dependencies",
		"optionalDependencies",
		"peerDependencies",
		"devDependencies",
	]) {
		assert.equal(
			Object.hasOwn(packageManifest, field),
			false,
			`package.json must not declare ${field}`,
		);
	}
});

test("raw artifact package owns an exact positive publication allowlist", () => {
	assert.deepEqual(
		packageManifest.files,
		REVIEWED_PACK_FILES.filter((path) => path !== "package.json"),
	);
});
