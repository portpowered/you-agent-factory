import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(
	dirname(fileURLToPath(import.meta.url)),
	"..",
);
const packageRoot = resolve(repositoryRoot, "packages", "packaged-factories");
const packageManifest = JSON.parse(
	await readFile(resolve(packageRoot, "package.json"), "utf8"),
);
const catalogManifest = JSON.parse(
	await readFile(resolve(packageRoot, "generated", "manifest.json"), "utf8"),
);

const expectedExports = {
	"./manifest": "./generated/manifest.json",
	"./schemas/factory.json": "./schemas/factory.schema.json",
	"./schemas/factory.yaml": "./schemas/factory.schema.yaml",
	"./factories/*.json": "./generated/factories/*/factory.json",
	"./factories/*.yaml": "./generated/factories/*/factory.yaml",
};

const expectedFiles = [
	"README.md",
	"LICENSE.md",
	"generated/manifest.json",
	"generated/factories",
	"schemas/factory.schema.json",
	"schemas/factory.schema.yaml",
];

test("packaged Factories exposes only the supported data subpaths", async () => {
	assert.equal(
		packageManifest.name,
		"@you-agent-factory/packaged-factories",
	);
	assert.deepEqual(packageManifest.exports, expectedExports);
	assert.deepEqual(packageManifest.files, expectedFiles);
	assert.deepEqual(packageManifest.publishConfig, { access: "public" });
	assert.equal(Object.hasOwn(packageManifest, "private"), false);

	for (const target of Object.values(expectedExports)) {
		assert.equal(target.startsWith("./"), true);
		assert.equal(target.includes("../"), false);
	}

	await Promise.all([
		access(resolve(packageRoot, "generated", "manifest.json")),
		access(resolve(packageRoot, "schemas", "factory.schema.json")),
		access(resolve(packageRoot, "schemas", "factory.schema.yaml")),
	]);
});

test("every manifest Factory has matching JSON and YAML public exports", async () => {
	assert.ok(catalogManifest.factories.length > 0);

	for (const factory of catalogManifest.factories) {
		const expectedJSON = expectedExports["./factories/*.json"].replace(
			"*",
			factory.slug,
		);
		const expectedYAML = expectedExports["./factories/*.yaml"].replace(
			"*",
			factory.slug,
		);

		assert.equal(`./${factory.json.locator}`, expectedJSON);
		assert.equal(`./${factory.yaml.locator}`, expectedYAML);
		await Promise.all([
			access(resolve(packageRoot, factory.json.locator)),
			access(resolve(packageRoot, factory.yaml.locator)),
		]);
	}
});

test("packaged Factories has no executable or runtime dependency surface", () => {
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

	assert.equal(
		packageManifest.files.some(
			(path) => path === "factories" || path.startsWith("factories/"),
		),
		false,
		"authored Factory sources must not be published",
	);
});
