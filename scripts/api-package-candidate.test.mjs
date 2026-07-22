import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
	DEVELOPMENT_DIST_TAG,
	SHORT_SHA_LENGTH,
	deriveCandidateVersion,
	prepareCandidate,
} from "./api-package-candidate.mjs";
import { REVIEWED_PACK_FILES } from "./api-package-pack.mjs";

const packageDirectory = fileURLToPath(
	new URL("../packages/api", import.meta.url),
);
const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

async function temporaryDirectory(t, name) {
	const directory = await mkdtemp(join(tmpdir(), name));
	t.after(() => rm(directory, { recursive: true, force: true }));
	return directory;
}

function digest(contents) {
	return `sha256:${createHash("sha256").update(contents).digest("hex")}`;
}

test("candidate version uses the immutable run ID and fixed commit prefix", () => {
	assert.equal(SHORT_SHA_LENGTH, 12);
	assert.equal(
		deriveCandidateVersion({
			baseVersion: "1.2.3",
			runId: "9876543210",
			sourceCommit,
		}),
		"1.2.3-dev.9876543210.0123456789ab",
	);
});

test("candidate identity rejects incomplete or non-canonical inputs", () => {
	const valid = { baseVersion: "1.2.3", runId: "42", sourceCommit };
	for (const replacement of [
		{ baseVersion: undefined },
		{ baseVersion: "1.2.3-beta.1" },
		{ baseVersion: "01.2.3" },
		{ runId: undefined },
		{ runId: "0" },
		{ runId: "042" },
		{ sourceCommit: undefined },
		{ sourceCommit: sourceCommit.slice(0, -1) },
		{ sourceCommit: sourceCommit.toUpperCase() },
	]) {
		assert.throws(() => deriveCandidateVersion({ ...valid, ...replacement }));
	}
});

test("preparation packs one attributable candidate without mutating package sources", async (t) => {
	const outputDirectory = await temporaryDirectory(t, "you-api-candidate-output-");
	const packageManifestPath = join(packageDirectory, "package.json");
	const contractManifestPath = join(
		packageDirectory,
		"generated",
		"manifest.json",
	);
	const packageManifestBefore = await readFile(packageManifestPath);
	const contractManifestBefore = await readFile(contractManifestPath);

	const result = await prepareCandidate({
		packageDirectory,
		outputDirectory,
		runId: "9876543210",
		sourceCommit,
	});

	assert.deepEqual(await readFile(packageManifestPath), packageManifestBefore);
	assert.deepEqual(await readFile(contractManifestPath), contractManifestBefore);
	assert.deepEqual(result.evidence, {
		packageName: "@you-agent-factory/api",
		candidateVersion: "0.0.2-dev.9876543210.0123456789ab",
		sourceCommit,
		contractDigest: digest(contractManifestBefore),
		artifactDigest: digest(await readFile(result.tarballPath)),
		inventory: [...REVIEWED_PACK_FILES].sort((left, right) =>
			left.localeCompare(right),
		),
		distTag: DEVELOPMENT_DIST_TAG,
	});
	assert.equal(
		await readFile(result.evidencePath, "utf8"),
		`${JSON.stringify(result.evidence, null, 2)}\n`,
	);
	assert.deepEqual(
		(await readdir(outputDirectory)).sort(),
		[basename(result.tarballPath), "candidate-evidence.json"].sort(),
	);
});

test("repeated preparation preserves identity, inventory, and artifact digest", async (t) => {
	const firstOutput = await temporaryDirectory(t, "you-api-candidate-first-");
	const secondOutput = await temporaryDirectory(t, "you-api-candidate-second-");
	const input = {
		packageDirectory,
		runId: "123456789",
		sourceCommit,
	};

	const first = await prepareCandidate({
		...input,
		outputDirectory: firstOutput,
	});
	const second = await prepareCandidate({
		...input,
		outputDirectory: secondOutput,
	});

	assert.deepEqual(second.evidence, first.evidence);
	assert.notEqual(second.tarballPath, first.tarballPath);
});

test("invalid identity fails before creating candidate output", async (t) => {
	const parent = await temporaryDirectory(t, "you-api-candidate-invalid-");
	const outputDirectory = join(parent, "candidate");

	await assert.rejects(
		prepareCandidate({
			packageDirectory,
			outputDirectory,
			runId: "not-a-run-id",
			sourceCommit,
		}),
		/run ID must be a canonical positive integer/,
	);
	await assert.rejects(readdir(outputDirectory), { code: "ENOENT" });
});

test("candidate evidence contains only the approved non-sensitive fields", async (t) => {
	const outputDirectory = await temporaryDirectory(t, "you-api-candidate-safe-");
	const secret = "never-include-this-npm-token";
	const previousNpmToken = process.env.NPM_TOKEN;
	process.env.NPM_TOKEN = secret;
	t.after(() => {
		if (previousNpmToken === undefined) {
			delete process.env.NPM_TOKEN;
			return;
		}
		process.env.NPM_TOKEN = previousNpmToken;
	});

	const result = await prepareCandidate({
		packageDirectory,
		outputDirectory,
		runId: "7",
		sourceCommit,
	});
	const serialized = JSON.stringify(result.evidence);

	assert.deepEqual(Object.keys(result.evidence), [
		"packageName",
		"candidateVersion",
		"sourceCommit",
		"contractDigest",
		"artifactDigest",
		"inventory",
		"distTag",
	]);
	assert.equal(serialized.includes(secret), false);
	assert.equal(serialized.includes("authorization"), false);
});
