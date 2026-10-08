import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { prepareTaggedReleaseCandidate } from "./public-release-package-candidate.mjs";
import {
	FRONTEND_PUBLIC_PACKAGE_NAMES,
	TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
} from "./public-package-set.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

test("tagged preparation represents exactly eight independently prepared packages at the protected identity", async (t) => {
	const outputDirectory = await mkdtemp(
		join(tmpdir(), "you-tagged-preparation-"),
	);
	t.after(() => rm(outputDirectory, { recursive: true, force: true }));
	const calls = [];
	const result = await prepareTaggedReleaseCandidate(
		{ outputDirectory, runId: "42", sourceCommit, version: "1.2.3" },
		{
			prepareData: async (input) => {
				calls.push(input);
				return {
					evidence: {
						packageName: input.packageName,
						candidateVersion: input.version,
					},
					tarballPath: join(input.outputDirectory, "candidate.tgz"),
				};
			},
			prepareFrontend: async (input) => {
				calls.push(input);
				return {
					packages: FRONTEND_PUBLIC_PACKAGE_NAMES.map((name, index) => ({
						name,
						version: input.version,
						filename: `frontend-${index}.tgz`,
					})),
				};
			},
		},
	);
	assert.equal(calls.length, 3);
	assert.equal(calls[0].sourceCommit, sourceCommit);
	assert.equal(calls[1].sourceCommit, sourceCommit);
	assert.equal(calls[0].distTag, "latest");
	assert.equal(calls[1].distTag, "latest");
	assert.deepEqual(
		result.evidence.packages.map(({ name }) => name),
		TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
	);
	assert.ok(
		result.evidence.packages.every(
			({ version, tarball }) =>
				version === "1.2.3" &&
				/^(api|packaged-factories|frontend)\/.+\.tgz$/.test(tarball),
		),
	);
	assert.equal(result.evidence.sourceCommit, sourceCommit);
	assert.deepEqual(
		JSON.parse(await readFile(result.evidencePath, "utf8")),
		result.evidence,
	);
});

test("a failed child preparation never emits a complete release candidate", async (t) => {
	const outputDirectory = await mkdtemp(
		join(tmpdir(), "you-tagged-preparation-"),
	);
	t.after(() => rm(outputDirectory, { recursive: true, force: true }));
	await assert.rejects(
		prepareTaggedReleaseCandidate(
			{ outputDirectory, runId: "42", sourceCommit, version: "1.2.3" },
			{
				prepareData: async () => {
					throw new Error("pack failed");
				},
				prepareFrontend: async () => ({ packages: [] }),
			},
		),
		/pack failed/,
	);
	await assert.rejects(
		readFile(join(outputDirectory, "release-candidate-evidence.json")),
		{ code: "ENOENT" },
	);
});
