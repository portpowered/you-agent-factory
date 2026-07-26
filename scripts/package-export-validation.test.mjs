import assert from "node:assert/strict";
import test from "node:test";

import {
	assertPackedExportTargets,
	assertPackedRequiredFiles,
} from "./package-export-validation.mjs";

const packageName = "@you-agent-factory/example";
const exports = {
	".": {
		types: "./dist/index.d.ts",
		import: "./dist/index.js",
		default: "./dist/index.js",
	},
	"./styles.css": "./dist/styles.css",
	"./joined/*": "./generated/joined/*",
};

test("complete concrete and wildcard export targets are accepted", () => {
	assert.doesNotThrow(() =>
		assertPackedExportTargets(packageName, exports, [
			{ path: "dist/index.d.ts" },
			{ path: "dist/index.js" },
			{ path: "dist/styles.css" },
			{ path: "generated/joined/contracts/manifest.schema.json" },
		]),
	);
});

test("a missing concrete export reports its package and target", () => {
	assert.throws(
		() =>
			assertPackedExportTargets(packageName, exports, [
				"dist/index.d.ts",
				"dist/index.js",
				"generated/joined/contracts/manifest.schema.json",
			]),
		{
			message: `${packageName} candidate omits export target dist/styles.css`,
		},
	);
});

test("an unmatched wildcard export reports its package and pattern", () => {
	assert.throws(
		() =>
			assertPackedExportTargets(packageName, exports, [
				"dist/index.d.ts",
				"dist/index.js",
				"dist/styles.css",
			]),
		{
			message: `${packageName} candidate omits export target generated/joined/*`,
		},
	);
});

test("required compatibility files are checked separately from exports", () => {
	const required = ["factories/goal/factory.json"];
	assert.doesNotThrow(() =>
		assertPackedRequiredFiles(packageName, required, [
			{ path: "factories/goal/factory.json" },
		]),
	);
	assert.throws(
		() =>
			assertPackedRequiredFiles(packageName, required, [
				{ path: "generated/factories/goal/factory.json" },
			]),
		{
			message: `${packageName} candidate omits factories/goal/factory.json`,
		},
	);
});
