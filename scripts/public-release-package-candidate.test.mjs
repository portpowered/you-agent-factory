import assert from "node:assert/strict";
import { access, mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
	TAGGED_RELEASE_CANDIDATE_SCOPE,
	prepareTaggedReleaseCandidate,
} from "./public-release-package-candidate.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

async function exists(path) {
	try {
		await access(path);
		return true;
	} catch (error) {
		if (error?.code === "ENOENT") return false;
		throw error;
	}
}

test("tagged release preparation stages each public package once at the requested version without changing source manifests", async (t) => {
	const outputDirectory = await mkdtemp(
		join(tmpdir(), "you-complete-release-candidate-"),
	);
	t.after(() => rm(outputDirectory, { recursive: true, force: true }));
	const manifests = [
		"packages/api/package.json",
		"packages/packaged-factories/package.json",
		"ui/packages/client/package.json",
		"ui/packages/factory-replay/package.json",
		"ui/packages/factory-emulator/package.json",
		"ui/packages/components/package.json",
		"ui/packages/factory-visualizers/package.json",
	];
	const before = await Promise.all(manifests.map((path) => readFile(path)));
	const buildOutputsBefore = await Promise.all(
		manifests.map((path) => exists(path.replace("package.json", "dist"))),
	);

	const result = await prepareTaggedReleaseCandidate({
		outputDirectory,
		runId: "42",
		sourceCommit,
		version: "1.2.3",
	});

	assert.equal(result.evidence.scope, TAGGED_RELEASE_CANDIDATE_SCOPE);
	assert.equal(result.evidence.version, "1.2.3");
	assert.equal(result.evidence.sourceCommit, sourceCommit);
	assert.equal(result.evidence.packages.length, 7);
	assert.equal(
		new Set(result.evidence.packages.map(({ name }) => name)).size,
		result.evidence.packages.length,
	);
	for (const candidate of result.evidence.packages) {
		assert.equal(candidate.version, "1.2.3");
		assert.match(candidate.tarball, /^(api|packaged-factories|frontend)\/.+\.tgz$/);
	}
	assert.deepEqual(
		await Promise.all(manifests.map((path) => readFile(path))),
		before,
	);
	assert.deepEqual(
		await Promise.all(
			manifests.map((path) => exists(path.replace("package.json", "dist"))),
		),
		buildOutputsBefore,
	);
	assert.ok((await readdir(outputDirectory)).includes("release-candidate-evidence.json"));
});
