import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
	access,
	cp,
	mkdir,
	mkdtemp,
	readFile,
	rm,
	symlink,
	unlink,
	writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
	packAndVerify,
	reviewedPackFiles,
} from "./packaged-factories-package-pack.mjs";

const packageDirectory = fileURLToPath(
	new URL("../packages/packaged-factories", import.meta.url),
);

async function temporaryDirectory(t, name) {
	const directory = await mkdtemp(join(tmpdir(), name));
	t.after(() => rm(directory, { recursive: true, force: true }));
	return directory;
}

async function fixturePackage(t) {
	const root = await temporaryDirectory(t, "you-factories-pack-fixture-");
	const packageRoot = join(root, "package");
	await cp(packageDirectory, packageRoot, { recursive: true });
	return { packageRoot, packDestination: root };
}

test("candidate packing suppresses lifecycle script side effects", async (t) => {
	const { packageRoot, packDestination } = await fixturePackage(t);
	const sentinel = join(packDestination, "lifecycle-ran");
	const manifestPath = join(packageRoot, "package.json");
	const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
	manifest.scripts = { prepack: "node create-sentinel.mjs" };
	await Promise.all([
		writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`),
		writeFile(
			join(packageRoot, "create-sentinel.mjs"),
			`import { writeFileSync } from "node:fs"; writeFileSync(${JSON.stringify(sentinel)}, "ran");\n`,
		),
	]);

	await packAndVerify({ packageDirectory: packageRoot, packDestination });
	await assert.rejects(access(sentinel), { code: "ENOENT" });
});

test("real npm tarball contains the manifest-derived reviewed inventory deterministically", async (t) => {
	const firstDestination = await temporaryDirectory(
		t,
		"you-factories-pack-first-",
	);
	const secondDestination = await temporaryDirectory(
		t,
		"you-factories-pack-second-",
	);
	const expected = await reviewedPackFiles(packageDirectory);

	const first = await packAndVerify({
		packageDirectory,
		packDestination: firstDestination,
	});
	const second = await packAndVerify({
		packageDirectory,
		packDestination: secondDestination,
	});

	assert.deepEqual(first.files, expected);
	assert.deepEqual(second.files, first.files);
	assert.deepEqual(
		await readFile(second.tarballPath),
		await readFile(first.tarballPath),
	);
	assert.notEqual(second.tarballPath, first.tarballPath);
});

test("missing and stale generated artifacts are reported separately", async (t) => {
	const { packageRoot, packDestination } = await fixturePackage(t);
	const manifest = JSON.parse(
		await readFile(join(packageRoot, "generated", "manifest.json"), "utf8"),
	);
	await unlink(
		join(packageRoot, ...manifest.factories[0].json.locator.split("/")),
	);
	const stale = join(
		packageRoot,
		"generated",
		"factories",
		"removed-factory",
		"factory.json",
	);
	await mkdir(dirname(stale), { recursive: true });
	await writeFile(stale, "{}\n");

	await assert.rejects(
		packAndVerify({ packageDirectory: packageRoot, packDestination }),
		(error) => {
			assert.match(
				error.message,
				/unexpected package files:\n {2}generated\/factories\/removed-factory\/factory\.json/,
			);
			assert.ok(
				error.message.includes(
					`missing package files:\n  ${manifest.factories[0].json.locator}`,
				),
			);
			return true;
		},
	);
});

test("escaping manifest locators and export targets fail before packing", async (t) => {
	for (const mutation of ["locator", "export"]) {
		const { packageRoot, packDestination } = await fixturePackage(t);
		if (mutation === "locator") {
			const path = join(packageRoot, "generated", "manifest.json");
			const manifest = JSON.parse(await readFile(path, "utf8"));
			manifest.factories[0].json.locator = "../outside.json";
			await writeFile(path, `${JSON.stringify(manifest, null, 2)}\n`);
		} else {
			const path = join(packageRoot, "package.json");
			const manifest = JSON.parse(await readFile(path, "utf8"));
			manifest.exports["./manifest"] = "../outside.json";
			await writeFile(path, `${JSON.stringify(manifest, null, 2)}\n`);
		}
		await assert.rejects(
			packAndVerify({ packageDirectory: packageRoot, packDestination }),
			/escapes the package|must target/,
		);
	}
});

test("symlinked artifacts are rejected", async (t) => {
	const { packageRoot, packDestination } = await fixturePackage(t);
	const manifest = JSON.parse(
		await readFile(join(packageRoot, "generated", "manifest.json"), "utf8"),
	);
	const artifactPath = join(
		packageRoot,
		...manifest.factories[0].json.locator.split("/"),
	);
	const contents = await readFile(artifactPath);
	const external = join(dirname(packageRoot), "external-factory.json");
	await writeFile(external, contents);
	await unlink(artifactPath);
	try {
		await symlink(external, artifactPath);
	} catch (error) {
		if (error?.code === "EPERM") {
			t.skip("the Windows test environment does not permit symlink creation");
			return;
		}
		throw error;
	}
	await assert.rejects(
		packAndVerify({ packageDirectory: packageRoot, packDestination }),
		/symlink/,
	);
});

test("repository-relative flattened artifact dependencies are rejected", async (t) => {
	const { packageRoot, packDestination } = await fixturePackage(t);
	const manifest = JSON.parse(
		await readFile(join(packageRoot, "generated", "manifest.json"), "utf8"),
	);
	const artifactPath = join(
		packageRoot,
		...manifest.factories[0].json.locator.split("/"),
	);
	const factory = JSON.parse(await readFile(artifactPath, "utf8"));
	factory.source = "../authored/factory.yaml";
	await writeFile(artifactPath, `${JSON.stringify(factory, null, 2)}\n`);

	await assert.rejects(
		packAndVerify({ packageDirectory: packageRoot, packDestination }),
		/package-external file dependencies/,
	);
});

test("missing Goal factory compatibility artifact is rejected separately", async (t) => {
	const { packageRoot, packDestination } = await fixturePackage(t);
	await unlink(join(packageRoot, "factories", "goal", "factory.json"));

	await assert.rejects(
		packAndVerify({ packageDirectory: packageRoot, packDestination }),
		{
			message:
				"@you-agent-factory/packaged-factories candidate omits factories/goal/factory.json",
		},
	);
});

test("npm report digest mismatch is rejected", async (t) => {
	const { packageRoot, packDestination } = await fixturePackage(t);
	const tarball = join(packDestination, "candidate.tgz");
	await writeFile(tarball, "candidate");
	const files = await reviewedPackFiles(packageRoot);
	const npmPack = async () =>
		JSON.stringify([
			{
				name: "@you-agent-factory/packaged-factories",
				version: "0.0.0",
				filename: "candidate.tgz",
				shasum: createHash("sha1").update("different").digest("hex"),
				files: files.map((path) => ({ path })),
			},
		]);

	await assert.rejects(
		packAndVerify({ packageDirectory: packageRoot, packDestination, npmPack }),
		/tarball digest mismatch/,
	);
});
