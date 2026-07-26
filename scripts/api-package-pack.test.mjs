import assert from "node:assert/strict";
import {
	access,
	cp,
	mkdir,
	mkdtemp,
	readFile,
	rm,
	unlink,
	writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { packAndVerify, REVIEWED_PACK_FILES } from "./api-package-pack.mjs";

const packageDirectory = fileURLToPath(
	new URL("../packages/api", import.meta.url),
);

async function temporaryDirectory(t, name) {
	const directory = await mkdtemp(join(tmpdir(), name));
	t.after(() => rm(directory, { recursive: true, force: true }));
	return directory;
}

test("candidate packing suppresses lifecycle script side effects", async (t) => {
	const fixtureRoot = await temporaryDirectory(t, "you-api-pack-lifecycle-");
	const fixturePackage = join(fixtureRoot, "package");
	const packDestination = join(fixtureRoot, "packed");
	const sentinel = join(fixtureRoot, "lifecycle-ran");
	await cp(packageDirectory, fixturePackage, { recursive: true });
	await mkdir(packDestination);

	const manifestPath = join(fixturePackage, "package.json");
	const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
	manifest.scripts = { prepack: "node create-sentinel.mjs" };
	await Promise.all([
		writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`),
		writeFile(
			join(fixturePackage, "create-sentinel.mjs"),
			`import { writeFileSync } from "node:fs"; writeFileSync(${JSON.stringify(sentinel)}, "ran");\n`,
		),
	]);

	await packAndVerify({
		packageDirectory: fixturePackage,
		packDestination,
	});
	await assert.rejects(access(sentinel), { code: "ENOENT" });
});

test("real npm tarball contains the complete reviewed inventory deterministically", async (t) => {
	const firstDestination = await temporaryDirectory(t, "you-api-pack-first-");
	const secondDestination = await temporaryDirectory(t, "you-api-pack-second-");

	const first = await packAndVerify({
		packageDirectory,
		packDestination: firstDestination,
	});
	const second = await packAndVerify({
		packageDirectory,
		packDestination: secondDestination,
	});

	assert.deepEqual(
		first.files,
		[...REVIEWED_PACK_FILES].sort((left, right) => left.localeCompare(right)),
	);
	assert.deepEqual(second.files, first.files);
	assert.notEqual(first.tarballPath, second.tarballPath);
});

test("real npm tarball reports every forbidden admitted path deterministically", async (t) => {
	const fixtureRoot = await temporaryDirectory(t, "you-api-pack-forbidden-");
	const fixturePackage = join(fixtureRoot, "package");
	const packDestination = join(fixtureRoot, "packed");
	await cp(packageDirectory, fixturePackage, { recursive: true });
	await mkdir(packDestination);

	const manifestPath = join(fixturePackage, "package.json");
	const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
	manifest.files = ["README.md", "LICENSE.md", "generated"];
	await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

	const forbiddenFiles = [
		"generated/cache/contracts/data.json",
		"generated/client/openapi.ts",
		"generated/components/foo.json",
		"generated/javascript/runtime.js",
		"generated/runtime/you.exe",
		"generated/ui/App.tsx",
		"generated/unrelated.json",
		"generated/validators/factory.js",
	];
	for (const path of forbiddenFiles) {
		const target = join(fixturePackage, ...path.split("/"));
		await mkdir(dirname(target), { recursive: true });
		await writeFile(target, path);
	}

	await assert.rejects(
		packAndVerify({ packageDirectory: fixturePackage, packDestination }),
		(error) => {
			const expected = [
				"[api-package-pack] tarball inventory rejected",
				"unexpected package files:",
				...forbiddenFiles.map((path) => `  ${path}`),
			].join("\n");
			assert.equal(error.message, expected);
			return true;
		},
	);
});

test("real npm tarball rejects a missing concrete export target", async (t) => {
	const fixtureRoot = await temporaryDirectory(t, "you-api-pack-concrete-");
	const fixturePackage = join(fixtureRoot, "package");
	await cp(packageDirectory, fixturePackage, { recursive: true });
	await unlink(join(fixturePackage, "generated", "openapi", "openapi.yaml"));

	await assert.rejects(
		packAndVerify({
			packageDirectory: fixturePackage,
			packDestination: fixtureRoot,
		}),
		{
			message:
				"@you-agent-factory/api candidate omits export target generated/openapi/openapi.yaml",
		},
	);
});

test("real npm tarball rejects an unmatched wildcard export target", async (t) => {
	const fixtureRoot = await temporaryDirectory(t, "you-api-pack-wildcard-");
	const fixturePackage = join(fixtureRoot, "package");
	await cp(packageDirectory, fixturePackage, { recursive: true });
	await rm(join(fixturePackage, "generated", "joined"), {
		recursive: true,
		force: true,
	});

	await assert.rejects(
		packAndVerify({
			packageDirectory: fixturePackage,
			packDestination: fixtureRoot,
		}),
		{
			message:
				"@you-agent-factory/api candidate omits export target generated/joined/*",
		},
	);
});
