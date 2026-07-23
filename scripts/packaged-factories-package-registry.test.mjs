import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	reconcileCandidate,
} from "./packaged-factories-package-registry.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

function digest(contents) {
	return `sha256:${createHash("sha256").update(contents).digest("hex")}`;
}

async function candidateFixture(t) {
	const directory = await mkdtemp(
		join(tmpdir(), "you-packaged-factories-registry-test-"),
	);
	t.after(() => rm(directory, { recursive: true, force: true }));
	const contents = Buffer.from("reviewed packaged factories candidate");
	const tarballPath = join(directory, "candidate.tgz");
	await writeFile(tarballPath, contents);
	return {
		contents,
		tarballPath,
		evidence: {
			packageName: "@you-agent-factory/packaged-factories",
			candidateVersion: "0.0.0-dev.42.0123456789ab",
			sourceCommit,
			contractDigest: digest("generated catalog"),
			artifactDigest: digest(contents),
			inventory: ["generated/manifest.json", "package.json"],
			distTag: "dev",
		},
	};
}

test("Packaged Factories reconciliation distinguishes absent and identical versions", async (t) => {
	const candidate = await candidateFixture(t);
	const absent = await reconcileCandidate({
		...candidate,
		registryClient: {
			async lookupVersion() {
				return { status: "absent" };
			},
			async downloadTarball() {
				assert.fail("absent versions are not downloaded");
			},
		},
	});
	assert.equal(absent.outcome, RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED);

	const existing = await reconcileCandidate({
		...candidate,
		registryClient: {
			async lookupVersion() {
				return { status: "present", tarballUrl: "controlled" };
			},
			async downloadTarball() {
				return candidate.contents;
			},
		},
	});
	assert.equal(existing.outcome, RECONCILIATION_OUTCOMES.VERIFIED_EXISTING);
});

test("Packaged Factories reconciliation rejects another package identity before registry IO", async (t) => {
	const candidate = await candidateFixture(t);
	candidate.evidence.packageName = "@you-agent-factory/api";
	let lookupCalled = false;
	await assert.rejects(
		reconcileCandidate({
			...candidate,
			registryClient: {
				async lookupVersion() {
					lookupCalled = true;
				},
				async downloadTarball() {},
			},
		}),
		(error) => error.code === RECONCILIATION_FAILURES.CANDIDATE_INVALID,
	);
	assert.equal(lookupCalled, false);
});

test("Packaged Factories reconciliation never accepts a different immutable tarball", async (t) => {
	const candidate = await candidateFixture(t);
	await assert.rejects(
		reconcileCandidate({
			...candidate,
			registryClient: {
				async lookupVersion() {
					return { status: "present", tarballUrl: "controlled" };
				},
				async downloadTarball() {
					return Buffer.from("different immutable candidate");
				},
			},
		}),
		(error) =>
			error.code === RECONCILIATION_FAILURES.IMMUTABLE_VERSION_CONFLICT,
	);
});
