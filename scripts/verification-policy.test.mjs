import assert from "node:assert/strict";
import test from "node:test";

import {
	evaluateVerificationPolicy,
	renderVerificationSummary,
} from "./verification-policy.mjs";

const laneNames = [
	"Docs Reference",
	"README",
	"Frontend",
	"Backend",
	"Backend Test Stability",
	"Backend Unit Latency",
	"Backend Lint",
	"Workflow Lint",
	"UI Backend Integration",
	"API Package",
	"Packaged Factories Package",
	"Model Providers Package",
];

function lane(name, selected = false, result = selected ? "success" : "skipped", options = {}) {
	return {
		name,
		selected: String(selected),
		reason: options.reason ?? (selected ? "Selected by the classifier." : "Not selected."),
		packageLane: options.packageLane ?? false,
		checks: options.checks ?? [{ name, result }],
	};
}

function policy(overrides = {}) {
	return {
		classificationResult: "success",
		classification: "documentation-reference",
		classificationReason: "Selected the union of verification lanes owned by the changed paths.",
		areas: "documentation-reference",
		packageWorkflowResult: "success",
		lanes: laneNames.map((name) => lane(name)),
		...overrides,
	};
}

test("minimal selected verification passes and unselected lanes may be skipped", () => {
	const result = evaluateVerificationPolicy(
		policy({
			lanes: [
				lane("Docs Reference", true, "success", {
					reason: "Documentation reference paths select this lane.",
				}),
				...laneNames.slice(1).map((name) => lane(name)),
			],
		}),
	);

	assert.deepEqual(result, { ok: true, failures: [] });
});

test("a selected lane that is skipped, missing, or failed fails closed", () => {
	for (const result of ["skipped", "", "failure"]) {
		const evaluation = evaluateVerificationPolicy(
			policy({
				lanes: [
					lane("Docs Reference", true, result),
					...laneNames.slice(1).map((name) => lane(name)),
				],
			}),
		);

		assert.equal(evaluation.ok, false, `result ${result || "missing"} must fail`);
		assert.match(evaluation.failures[0], /Docs Reference was selected/);
	}
});

test("required Backend Lint fails the policy when its hosted job is skipped", () => {
	for (const result of ["skipped", "cancelled", "timed_out", "failure"]) {
		const evaluation = evaluateVerificationPolicy(
			policy({
				lanes: [
					...laneNames.filter((name) => name !== "Backend Lint").map((name) => lane(name)),
					lane("Backend Lint", true, result, {
						reason: "The canonical lint inventory is required on every pull request.",
					}),
				],
			}),
		);

		assert.equal(evaluation.ok, false, `${result} must fail the required policy lane`);
		assert.ok(evaluation.failures.some((failure) => /Backend Lint was selected/.test(failure)));
	}
});

test("required Backend Unit Latency fails closed for every incomplete hosted result", () => {
	for (const result of ["skipped", "", "cancelled", "timed_out", "failure"]) {
		const evaluation = evaluateVerificationPolicy(
			policy({
				lanes: [
					...laneNames.filter((name) => name !== "Backend Unit Latency").map((name) => lane(name)),
					lane("Backend Unit Latency", true, result, {
						reason: "The canonical three-sample unit-lane latency budget is required on every pull request and push to main.",
					}),
				],
			}),
		);

		assert.equal(evaluation.ok, false, `${result || "missing"} must fail the required latency gate`);
		assert.ok(
			evaluation.failures.some((failure) => /Backend Unit Latency was selected/.test(failure)),
		);
	}
});

test("changed-test stability passes when selected, accepts a classifier-driven skip, and fails incomplete outcomes", () => {
	const selectedPass = evaluateVerificationPolicy(
		policy({
			lanes: [
				...laneNames
					.filter((name) => name !== "Backend Test Stability")
					.map((name) => lane(name)),
				lane("Backend Test Stability", true, "success", {
					reason: "Backend changes select the merge-base stability gate.",
				}),
			],
		}),
	);
	assert.deepEqual(selectedPass, { ok: true, failures: [] });

	const validSkip = evaluateVerificationPolicy(
		policy({
			classification: "factory-content",
			areas: "factory-content",
			lanes: [
				...laneNames
					.filter((name) => name !== "Backend Test Stability")
					.map((name) => lane(name)),
				lane("Backend Test Stability", false, "skipped", {
					reason: "Factory-only changes do not select backend verification.",
				}),
			],
		}),
	);
	assert.deepEqual(validSkip, { ok: true, failures: [] });

	for (const result of ["failure", "cancelled", "skipped", "", "incomplete"]) {
		const evaluation = evaluateVerificationPolicy(
			policy({
				lanes: [
					...laneNames
						.filter((name) => name !== "Backend Test Stability")
						.map((name) => lane(name)),
					lane("Backend Test Stability", true, result),
				],
			}),
		);

		assert.equal(evaluation.ok, false, `${result || "missing"} must fail the required stability gate`);
		assert.ok(
			evaluation.failures.some((failure) => /Backend Test Stability was selected/.test(failure)),
		);
	}
});

test("required Workflow Lint fails the policy when its hosted job is skipped or fails", () => {
	for (const result of ["skipped", "cancelled", "timed_out", "failure"]) {
		const evaluation = evaluateVerificationPolicy(
			policy({
				lanes: [
					...laneNames.filter((name) => name !== "Workflow Lint").map((name) => lane(name)),
					lane("Workflow Lint", true, result, {
						reason: "Every repository workflow must pass schema-aware lint.",
					}),
				],
			}),
		);

		assert.equal(evaluation.ok, false, `${result} must fail the required workflow lint lane`);
		assert.ok(evaluation.failures.some((failure) => /Workflow Lint was selected/.test(failure)));
	}
});

test("classifier failure fails policy even when every product lane succeeds", () => {
	const evaluation = evaluateVerificationPolicy(
		policy({ classificationResult: "failure" }),
	);

	assert.equal(evaluation.ok, false);
	assert.match(evaluation.failures[0], /Classification did not complete successfully/);
});

test("reusable package failure and missing selected candidate fail policy", () => {
	const evaluation = evaluateVerificationPolicy(
		policy({
			packageWorkflowResult: "failure",
			lanes: [
				...laneNames.slice(0, 5).map((name) => lane(name)),
				lane("API Package", true, "success", {
					packageLane: true,
					checks: [
						{ name: "verification", result: "success", allowMissingWhenNotRequired: true },
						{
							name: "candidate",
							result: "",
							required: true,
							allowMissingWhenNotRequired: true,
						},
					],
				}),
				lane("Packaged Factories Package"),
				lane("Model Providers Package"),
			],
		}),
	);

	assert.equal(evaluation.ok, false);
	assert.ok(evaluation.failures.some((failure) => /Development Package/.test(failure)));
	assert.ok(evaluation.failures.some((failure) => /API Package \/ candidate/.test(failure)));
});

test("unselected package outputs may be absent, but an unexpected unselected result fails", () => {
	const packageChecks = [
		{
			name: "verification",
			result: "",
			allowMissingWhenNotRequired: true,
		},
		{
			name: "candidate",
			result: "",
			required: false,
			allowMissingWhenNotRequired: true,
		},
	];
	const passing = evaluateVerificationPolicy(
		policy({
			lanes: [
				...laneNames.slice(0, 5).map((name) => lane(name)),
				lane("API Package", false, "", {
					packageLane: true,
					checks: packageChecks,
				}),
				lane("Packaged Factories Package"),
				lane("Model Providers Package"),
			],
		}),
	);
	assert.equal(passing.ok, true);

	const unexpected = evaluateVerificationPolicy(
		policy({
			lanes: [
				lane("Docs Reference", false, "success"),
				...laneNames.slice(1).map((name) => lane(name)),
			],
		}),
	);
	assert.equal(unexpected.ok, false);
	assert.match(unexpected.failures[0], /not selected but returned success/);
});

test("an unselected package verification lane may omit its reusable-workflow output", () => {
	const evaluation = evaluateVerificationPolicy(
		policy({
			classification: "factory-content",
			areas: "factory-content",
			packageWorkflowResult: "skipped",
			lanes: [
				...laneNames.slice(0, 7).map((name) => lane(name)),
				lane("Model Providers Package", false, "", {
					checks: [
						{
							name: "Model Providers Package",
							result: "",
							allowMissingWhenNotRequired: true,
						},
					],
				}),
			],
		}),
	);

	assert.deepEqual(evaluation, { ok: true, failures: [] });
});

test("summary records touched areas, each decision, reason, and terminal result", () => {
	const input = policy({
		areas: "documentation-reference+frontend",
		lanes: [
			lane("Docs Reference", true, "success", {
				reason: "docs/reference paths select the documentation lane.",
			}),
			...laneNames.slice(1).map((name) => lane(name)),
		],
	});
	const evaluation = evaluateVerificationPolicy(input);
	const summary = renderVerificationSummary({ ...input, evaluation });

	assert.match(summary, /Areas touched: `documentation-reference\+frontend`/);
	assert.match(summary, /\| Docs Reference \| `run` \| Docs Reference: success \| docs\/reference paths select/);
	assert.match(summary, /\| README \| `skip` \| README: skipped \|/);
	assert.match(summary, /Verification Policy passed/);
});

test("a successful coverage lane stays successful when its reason contains advisory findings", () => {
	const evaluation = evaluateVerificationPolicy(
		policy({
			classification: "backend",
			areas: "ci-tooling",
			lanes: [
				...laneNames.filter((name) => name !== "Backend").map((name) => lane(name)),
				lane("Backend", true, "success", {
					reason:
						"COVERAGE FLOOR POLICY: advisory; package coverage regression and missing-manifest findings are report-only.",
				}),
			],
		}),
	);

	assert.deepEqual(evaluation, { ok: true, failures: [] });
});

test("summary falls back to the classifier's global reason for a selected lane", () => {
	const input = policy({
		classificationReason: "Unknown paths require conservative full verification.",
		lanes: [
			lane("Docs Reference", true, "success", { reason: "" }),
			...laneNames.slice(1).map((name) => lane(name)),
		],
	});
	const evaluation = evaluateVerificationPolicy(input);
	const summary = renderVerificationSummary({ ...input, evaluation });

	assert.match(summary, /\| Docs Reference \| `run` \| Docs Reference: success \| Unknown paths require/);
});
