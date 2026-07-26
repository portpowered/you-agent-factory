import assert from "node:assert/strict";
import test from "node:test";

import {
	assertCandidateSetEvidence,
	FRONTEND_ONLY_CANDIDATE_SCOPE,
	FRONTEND_PUBLIC_PACKAGE_NAMES,
	TAGGED_RELEASE_CANDIDATE_SCOPE,
	TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
} from "./public-package-set.mjs";

function evidence(scope, names, version = "1.2.3") {
	return {
		scope,
		version,
		packages: names.map((name) => ({ name, version })),
	};
}

test("accepts the exact package family selected by each known scope", () => {
	assert.deepEqual(
		assertCandidateSetEvidence(
			evidence(FRONTEND_ONLY_CANDIDATE_SCOPE, FRONTEND_PUBLIC_PACKAGE_NAMES),
			FRONTEND_ONLY_CANDIDATE_SCOPE,
		),
		FRONTEND_PUBLIC_PACKAGE_NAMES,
	);
	assert.deepEqual(
		assertCandidateSetEvidence(
			evidence(
				TAGGED_RELEASE_CANDIDATE_SCOPE,
				TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
			),
			TAGGED_RELEASE_CANDIDATE_SCOPE,
		),
		TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
	);
});

test("rejects unknown candidate scopes", () => {
	assert.throws(
		() => assertCandidateSetEvidence(evidence("all-packages", [])),
		/unknown candidate scope/,
	);
});

test("rejects duplicate package names", () => {
	const candidate = evidence(
		FRONTEND_ONLY_CANDIDATE_SCOPE,
		FRONTEND_PUBLIC_PACKAGE_NAMES,
	);
	candidate.packages[1].name = candidate.packages[0].name;
	assert.throws(
		() => assertCandidateSetEvidence(candidate),
		/duplicate package names/,
	);
});

test("rejects missing packages and mismatched candidate counts", () => {
	const candidate = evidence(
		TAGGED_RELEASE_CANDIDATE_SCOPE,
		TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES.slice(0, -1),
	);
	assert.throws(
		() => assertCandidateSetEvidence(candidate),
		/candidate count mismatch/,
	);
});

test("rejects a package whose version differs from the coherent set", () => {
	const candidate = evidence(
		FRONTEND_ONLY_CANDIDATE_SCOPE,
		FRONTEND_PUBLIC_PACKAGE_NAMES,
	);
	candidate.packages[0].version = "1.2.4";
	assert.throws(
		() => assertCandidateSetEvidence(candidate),
		/missing exact candidate/,
	);
});

test("requires a stable version for a tagged release set", () => {
	assert.throws(
		() =>
			assertCandidateSetEvidence(
				evidence(
					TAGGED_RELEASE_CANDIDATE_SCOPE,
					TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
					"1.2.3-rc.1",
				),
			),
		/invalid tagged-release candidate version/,
	);
});
