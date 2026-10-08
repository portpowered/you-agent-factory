import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
	evaluateVerificationPolicy,
	renderVerificationSummary,
} from "./verification-policy.mjs";

const laneNames = [
	"Docs Reference",
	"README",
	"Frontend",
	"Backend",
	"Backend Models Wire and Race",
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

test("queue coverage and lint producers require successful results", () => {
	const producers = ["Backend Coverage", "Backend Lint"];
	const queue = (name, result) => policy({ lanes: producers.map((producer) => lane(producer, true, producer === name ? result : "success")) });
	assert.deepEqual(evaluateVerificationPolicy(queue("", "success")), { ok: true, failures: [] });
	for (const name of producers) {
		for (const result of ["", "skipped", "failure", "cancelled", "timed_out"]) {
			const evaluation = evaluateVerificationPolicy(queue(name, result));
			assert.equal(evaluation.ok, false, `${name}: ${result}`);
			assert.ok(evaluation.failures.some((failure) => failure.includes(name)));
		}
	}
	assert.deepEqual(evaluateVerificationPolicy(policy({ lanes: [lane("Backend Coverage", true), lane("Backend Lint", true), lane("Backend", false)] })), { ok: true, failures: [] });
});

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

test("unit-only PR accepts unselected skips and fails every mandatory or selected result independently", () => {
	const env = { ...process.env, GITHUB_STEP_SUMMARY: "", CLASSIFICATION_RESULT: "success",
		CLASSIFICATION: "documentation", PACKAGE_WORKFLOW_RESULT: "skipped", RUN_CANDIDATES: "true", API_INDEPENDENT: "true" };
	for (const prefix of ["DOCS", "README", "FRONTEND", "BACKEND", "BACKEND_LINT", "WORKFLOW_LINT", "UI_BACKEND", "API", "PACKAGED", "PROVIDERS"]) {
		env[`RUN_${prefix}`] = "false";
		env[`${prefix}_RESULT`] = "skipped";
	}
	for (const suffix of ["COMPONENT", "COVERAGE", "BROWSER", "STORYBOOK"]) env[`FRONTEND_${suffix}_RESULT`] = "skipped";
	for (const key of ["BACKEND_MODELS_RESULT", "BACKEND_TTS_RESULT", "API_CANDIDATE_RESULT", "PACKAGED_CANDIDATE_RESULT"]) env[key] = "skipped";
	for (const prefix of ["DOCS", "README", "BACKEND_COVERAGE", "BACKEND_LINT", "WORKFLOW_LINT"]) {
		env[`RUN_${prefix}`] = "true";
		env[`${prefix}_RESULT`] = "success";
	}
	const invoke = (values) => spawnSync(process.execPath, [fileURLToPath(new URL("./verification-policy.mjs", import.meta.url))], { encoding: "utf8", env: values });
	assert.equal(invoke(env).status, 0);
	for (const key of ["CLASSIFICATION_RESULT", "DOCS_RESULT", "README_RESULT", "BACKEND_COVERAGE_RESULT", "BACKEND_LINT_RESULT", "WORKFLOW_LINT_RESULT"]) {
		for (const result of ["", "skipped", "failure", "cancelled"]) assert.equal(invoke({ ...env, [key]: result }).status, 1, key);
	}
	const selected = { ...env, RUN_BACKEND: "true", BACKEND_RESULT: "success", BACKEND_TTS_RESULT: "success", BACKEND_MODELS_RESULT: "success" };
	assert.equal(invoke(selected).status, 0);
	for (const key of ["BACKEND_RESULT", "BACKEND_TTS_RESULT", "BACKEND_MODELS_RESULT"]) {
		for (const result of ["", "skipped", "failure", "cancelled"]) assert.equal(invoke({ ...selected, [key]: result }).status, 1, key);
	}
});

test("API-only PR requires both independent proofs and permits skipped reusable children", () => {
	const base = { ...process.env, GITHUB_STEP_SUMMARY: "", BACKEND_COVERAGE_RESULT: "success", RUN_BACKEND_COVERAGE: "true", BACKEND_TTS_RESULT: "skipped", CLASSIFICATION_RESULT: "success",
		CLASSIFICATION: "api-package", PACKAGE_WORKFLOW_RESULT: "skipped", RUN_CANDIDATES: "true", API_INDEPENDENT: "true" };
	for (const prefix of ["DOCS", "README", "FRONTEND", "BACKEND", "BACKEND_LINT", "WORKFLOW_LINT", "UI_BACKEND", "API", "PACKAGED", "PROVIDERS"]) {
		base[`RUN_${prefix}`] = "false";
		base[`${prefix}_RESULT`] = "skipped";
	}
	for (const suffix of ["COMPONENT", "COVERAGE", "BROWSER", "STORYBOOK"]) base[`FRONTEND_${suffix}_RESULT`] = "skipped";
	Object.assign(base, { BACKEND_MODELS_RESULT: "skipped", API_CANDIDATE_RESULT: "success",
		RUN_API: "true", API_RESULT: "success", PACKAGED_CANDIDATE_RESULT: "skipped" });
	const invoke = (env) => spawnSync(process.execPath, [fileURLToPath(new URL("./verification-policy.mjs", import.meta.url))], { encoding: "utf8", env });
	assert.equal(invoke(base).status, 0);
	for (const key of ["API_RESULT", "API_CANDIDATE_RESULT"]) {
		for (const result of ["failure", "cancelled", "timed_out", "skipped", ""]) {
			const child = invoke({ ...base, [key]: result });
			assert.equal(child.status, 1);
			assert.match(child.stderr, /API Package/);
		}
	}
	for (const result of ["failure", "cancelled", "timed_out", ""]) assert.equal(invoke({ ...base, PACKAGE_WORKFLOW_RESULT: result }).status, 1);
	assert.equal(invoke({ ...base, API_INDEPENDENT: "false" }).status, 1);
	assert.equal(invoke({ ...base, RUN_PACKAGED: "true", PACKAGED_RESULT: "success", PACKAGED_CANDIDATE_RESULT: "success" }).status, 1);
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

test("selected Models verification requires successful hosted Wire and race results", () => {
	for (const result of ["success", "skipped", "", "cancelled", "timed_out", "failure"]) {
		const evaluation = evaluateVerificationPolicy(
			policy({
				lanes: [
					lane("Backend", true, "success"),
					lane("Backend Models Wire and Race", true, result),
				],
			}),
		);
		assert.equal(evaluation.ok, result === "success");
		if (result !== "success") {
			assert.match(evaluation.failures[0], /Backend Models Wire and Race was selected/);
		}
	}
});

test("policy CLI consumes the hosted Models result independently of backend coverage", () => {
	const env = {
		...process.env,
		GITHUB_STEP_SUMMARY: "", BACKEND_COVERAGE_RESULT: "success", RUN_BACKEND_COVERAGE: "true", BACKEND_TTS_RESULT: "skipped",
		CLASSIFICATION_RESULT: "success",
		CLASSIFICATION: "backend",
		PACKAGE_WORKFLOW_RESULT: "skipped",
		RUN_CANDIDATES: "false",
	};
	for (const prefix of ["DOCS", "README", "FRONTEND", "BACKEND", "BACKEND_LINT", "WORKFLOW_LINT", "UI_BACKEND", "API", "PACKAGED", "PROVIDERS"]) {
		env[`RUN_${prefix}`] = "false";
		env[`${prefix}_RESULT`] = "skipped";
	}
	for (const suffix of ["COMPONENT", "COVERAGE", "BROWSER", "STORYBOOK"]) {
		env[`FRONTEND_${suffix}_RESULT`] = "skipped";
	}
	env.RUN_BACKEND = "true";
	env.BACKEND_RESULT = "success";
	env.BACKEND_TTS_RESULT = "success";
	for (const result of ["success", "failure", "skipped", ""]) {
		const child = spawnSync(process.execPath, [fileURLToPath(new URL("./verification-policy.mjs", import.meta.url))], {
			encoding: "utf8",
			env: { ...env, BACKEND_MODELS_RESULT: result },
		});
		assert.equal(child.status, result === "success" ? 0 : 1, child.stderr);
		if (result !== "success") {
			assert.match(child.stderr, /Backend Models Wire and Race was selected/);
		}
	}
});

test("policy CLI fails closed for shared Frontend proof and independent frontend jobs", () => {
	const env = { ...process.env, GITHUB_STEP_SUMMARY: "", BACKEND_COVERAGE_RESULT: "success", RUN_BACKEND_COVERAGE: "true", BACKEND_TTS_RESULT: "skipped",
		CLASSIFICATION_RESULT: "success", CLASSIFICATION: "frontend", BACKEND_MODELS_RESULT: "skipped",
		PACKAGE_WORKFLOW_RESULT: "skipped", RUN_CANDIDATES: "false" };
	for (const prefix of ["DOCS", "README", "FRONTEND", "BACKEND", "BACKEND_CONFORMANCE",
		"BACKEND_LINT", "WORKFLOW_LINT", "UI_BACKEND", "API", "PACKAGED", "PROVIDERS"]) {
		env[`RUN_${prefix}`] = "false";
		env[`${prefix}_RESULT`] = "skipped";
	}
	const results = ["success", "failure", "cancelled", "timed_out", "skipped", ""];
	for (const selected of ["true", "false"]) {
		for (const result of results) {
			const childEnv = { ...env, RUN_FRONTEND: selected,
				FRONTEND_RESULT: result, FRONTEND_COVERAGE_RESULT: result };
			for (const suffix of ["COMPONENT", "BROWSER", "STORYBOOK"]) {
				childEnv[`FRONTEND_${suffix}_RESULT`] = selected === "true" ? "success" : "skipped";
			}
			const child = spawnSync(process.execPath,
				[fileURLToPath(new URL("./verification-policy.mjs", import.meta.url))],
				{ encoding: "utf8", env: childEnv });
			const passes = selected === "true" ? result === "success" : result === "skipped";
			assert.equal(child.status, passes ? 0 : 1, child.stderr);
			if (!passes) assert.match(child.stderr, /Frontend/);
		}
	}
	for (const suffix of ["COMPONENT", "BROWSER", "STORYBOOK"]) {
		for (const result of results.filter((value) => value !== "success")) {
			const childEnv = { ...env, RUN_FRONTEND: "true", FRONTEND_RESULT: "success",
				FRONTEND_COVERAGE_RESULT: "success", FRONTEND_COMPONENT_RESULT: "success",
				FRONTEND_BROWSER_RESULT: "success", FRONTEND_STORYBOOK_RESULT: "success",
				[`FRONTEND_${suffix}_RESULT`]: result };
			const child = spawnSync(process.execPath,
				[fileURLToPath(new URL("./verification-policy.mjs", import.meta.url))],
				{ encoding: "utf8", env: childEnv });
			assert.equal(child.status, 1, child.stderr);
			assert.match(child.stderr, new RegExp(`Frontend ${suffix[0]}${suffix.slice(1).toLowerCase()} was selected`));
		}
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
	assert.doesNotMatch(summary, /Backend Test Stability/);
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
