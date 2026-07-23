import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { prepareCandidate as prepareApiCandidate } from "./api-package-candidate.mjs";
import {
	DEVELOPMENT_DIST_TAG,
	prepareCandidate,
} from "./packaged-factories-package-candidate.mjs";

const packageDirectory = fileURLToPath(
	new URL("../packages/packaged-factories", import.meta.url),
);
const apiPackageDirectory = fileURLToPath(
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

function stagedContractManifest(contents) {
	return `${JSON.stringify(
		{ ...JSON.parse(contents), sourceCommit },
		null,
		2,
	)}\n`;
}

function expectedInventory(contractManifest) {
	const artifactFiles = contractManifest.factories.flatMap((factory) => [
		factory.json.locator,
		factory.yaml.locator,
	]);
	return [
		"LICENSE.md",
		"README.md",
		"generated/README.md",
		"generated/manifest.json",
		"package.json",
		"schemas/factory.schema.json",
		"schemas/factory.schema.yaml",
		...artifactFiles,
	].sort((left, right) => left.localeCompare(right));
}

test("Packaged Factories candidate is attributable and leaves package sources unchanged", async (t) => {
	const outputDirectory = await temporaryDirectory(
		t,
		"you-packaged-factories-candidate-",
	);
	const packageManifestPath = join(packageDirectory, "package.json");
	const contractManifestPath = join(
		packageDirectory,
		"generated",
		"manifest.json",
	);
	const packageManifestBefore = await readFile(packageManifestPath);
	const contractManifestBefore = await readFile(contractManifestPath);
	const contractManifest = JSON.parse(contractManifestBefore);

	const result = await prepareCandidate({
		packageDirectory,
		outputDirectory,
		runId: "9876543210",
		sourceCommit,
	});

	assert.deepEqual(await readFile(packageManifestPath), packageManifestBefore);
	assert.deepEqual(await readFile(contractManifestPath), contractManifestBefore);
	assert.deepEqual(result.evidence, {
		packageName: "@you-agent-factory/packaged-factories",
		candidateVersion: "0.0.0-dev.9876543210.0123456789ab",
		sourceCommit,
		contractDigest: digest(stagedContractManifest(contractManifestBefore)),
		artifactDigest: digest(await readFile(result.tarballPath)),
		inventory: expectedInventory(contractManifest),
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

test("API and Packaged Factories candidates share release identity and provenance", async (t) => {
	const apiOutput = await temporaryDirectory(t, "you-shared-api-candidate-");
	const factoriesOutput = await temporaryDirectory(
		t,
		"you-shared-factories-candidate-",
	);
	const input = { runId: "42", sourceCommit };

	const api = await prepareApiCandidate({
		...input,
		packageDirectory: apiPackageDirectory,
		outputDirectory: apiOutput,
	});
	const factories = await prepareCandidate({
		...input,
		packageDirectory,
		outputDirectory: factoriesOutput,
	});

	assert.equal(factories.evidence.candidateVersion, api.evidence.candidateVersion);
	assert.equal(factories.evidence.sourceCommit, api.evidence.sourceCommit);
	assert.equal(factories.evidence.distTag, api.evidence.distTag);
});

test("repeated preparation preserves Packaged Factories candidate evidence", async (t) => {
	const firstOutput = await temporaryDirectory(t, "you-factories-first-");
	const secondOutput = await temporaryDirectory(t, "you-factories-second-");
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

test("invalid shared identity fails before creating candidate output", async (t) => {
	const parent = await temporaryDirectory(t, "you-factories-invalid-");
	const outputDirectory = join(parent, "candidate");

	await assert.rejects(
		prepareCandidate({
			packageDirectory,
			outputDirectory,
			runId: "042",
			sourceCommit,
		}),
		/run ID must be a canonical positive integer/,
	);
	await assert.rejects(readdir(outputDirectory), { code: "ENOENT" });
});
