import assert from "node:assert/strict";
import test from "node:test";
import { executePackageWorkflow } from "./public-package-workflow.mjs";

import {
	assertWorkflowBehaviorEnvironment,
	workflowBehaviorScenarios,
} from "./development-package-workflow-behavior.mjs";

function environmentForScenario(scenario, workflowResult) {
	return {
		[`${scenario.envPrefix}_WORKFLOW_RESULT`]:
			workflowResult ?? scenario.workflowResults[0],
		...Object.fromEntries(
			Object.entries(scenario.outputs).map(([name, result]) => [
				`${scenario.envPrefix}_${name.toUpperCase()}`,
				result,
			]),
		),
	};
}

function environmentForAllScenarios(includeFailure) {
	return Object.assign(
		{ INCLUDE_FAILURE: String(includeFailure) },
		...workflowBehaviorScenarios.map((scenario) => {
			if (scenario.name === "selected package failure" && !includeFailure) {
				return {
					[`${scenario.envPrefix}_WORKFLOW_RESULT`]: "skipped",
					...Object.fromEntries(
						Object.keys(scenario.outputs).map((name) => [
							`${scenario.envPrefix}_${name.toUpperCase()}`,
							"",
						]),
					),
				};
			}
			return environmentForScenario(scenario);
		}),
	);
}

test("the hosted assertion harness covers API-only, mixed, no-package, and protected-main behavior", () => {
	assertWorkflowBehaviorEnvironment(environmentForAllScenarios(false));
});

test("the hosted assertion harness accepts selected package failure propagation", () => {
	assertWorkflowBehaviorEnvironment(environmentForAllScenarios(true));
});

test("scenario fixtures expose every reusable workflow output", () => {
	const names = new Set(
		workflowBehaviorScenarios.flatMap((scenario) =>
			Object.keys(scenario.outputs),
		),
	);

	assert.deepEqual([...names].sort(), [
		"api_candidate_result",
		"api_package_result",
		"model_providers_package_result",
		"packaged_factories_candidate_result",
		"packaged_factories_package_result",
	]);
});

const sourceCommit = "a".repeat(40);
const workflowInputs = {
	is_ci_call: true,
	run_candidates: true,
	run_api_package: true,
	run_packaged_factories_package: true,
	run_model_providers_package: true,
	source_commit: sourceCommit,
	pull_request_head_sha: sourceCommit,
	source_ref: "refs/pull/42/head",
	repository: "portpowered/you-agent-factory",
};

function runner(failure = "") {
	const calls = [];
	return {
		calls,
		runId: "42",
		log() {},
		async prepare(family, input) {
			calls.push(["prepare", family.key, input.sourceCommit]);
			if (failure === `${family.key}-prepare`)
				throw new Error("prepare failed");
			return { identity: family.key };
		},
		async verify(family, prepared) {
			assert.equal(prepared.identity, family.key);
			calls.push(["verify", family.key]);
			if (failure === `${family.key}-verify`)
				throw new Error("consumer failed");
		},
	};
}

test("selected artifacts are prepared and installed once for all five independent results", async () => {
	const dependencies = runner();
	const result = await executePackageWorkflow(workflowInputs, dependencies);
	assert.equal(result.exitCode, 0);
	assert.deepEqual(result.outputs, workflowBehaviorScenarios[1].outputs);
	assert.equal(dependencies.calls.length, 6);
	for (const family of ["api", "packaged", "providers"]) {
		assert.deepEqual(
			dependencies.calls.filter((call) => call[1] === family),
			[
				["prepare", family, sourceCommit],
				["verify", family],
			],
		);
	}
});

test("no selection and non-CI invocations have no preparation or consumer effects", async () => {
	for (const inputs of [
		{
			...workflowInputs,
			run_api_package: false,
			run_packaged_factories_package: false,
			run_model_providers_package: false,
		},
		{ ...workflowInputs, is_ci_call: false },
	]) {
		const dependencies = runner();
		const result = await executePackageWorkflow(inputs, dependencies);
		assert.equal(result.exitCode, 0);
		assert.deepEqual(result.outputs, workflowBehaviorScenarios[2].outputs);
		assert.deepEqual(dependencies.calls, []);
	}
});

test("every preparation and installed-consumer failure preserves other families' success", async () => {
	for (const family of ["api", "packaged", "providers"]) {
		for (const phase of ["prepare", "verify"]) {
			const dependencies = runner(`${family}-${phase}`);
			const { outputs, exitCode } = await executePackageWorkflow(
				workflowInputs,
				dependencies,
			);
			assert.equal(exitCode, 1);
			const failedNames = Object.entries(outputs)
				.filter(([, result]) => result === "failure")
				.map(([name]) => name);
			assert.deepEqual(
				failedNames,
				family === "api"
					? ["api_package_result", "api_candidate_result"]
					: family === "packaged"
						? [
								"packaged_factories_package_result",
								"packaged_factories_candidate_result",
							]
						: ["model_providers_package_result"],
			);
			for (const [name, result] of Object.entries(outputs))
				if (!failedNames.includes(name)) assert.equal(result, "success");
			if (phase === "prepare")
				assert.ok(
					!dependencies.calls.some(
						([operation, key]) => operation === "verify" && key === family,
					),
				);
		}
	}
});

test("candidate policy failure does not suppress package verification or rebuild its artifact", async () => {
	const dependencies = runner();
	const { outputs, exitCode } = await executePackageWorkflow(
		{ ...workflowInputs, pull_request_head_sha: "b".repeat(40) },
		dependencies,
	);
	assert.equal(exitCode, 1);
	assert.equal(outputs.api_candidate_result, "failure");
	assert.equal(outputs.packaged_factories_candidate_result, "failure");
	assert.equal(outputs.api_package_result, "success");
	assert.equal(outputs.packaged_factories_package_result, "success");
	assert.equal(dependencies.calls.length, 6);
});

test("disabled candidates remain empty while selected packages pass", async () => {
	const { outputs, exitCode } = await executePackageWorkflow(
		{ ...workflowInputs, run_candidates: false },
		runner(),
	);
	assert.equal(exitCode, 0);
	assert.deepEqual(outputs, workflowBehaviorScenarios[3].outputs);
});

test("hosted probes preserve independently failed package and candidate results without effects", async () => {
	for (const lane of [
		"",
		"api",
		"api_candidate",
		"packaged",
		"packaged_candidate",
		"providers",
	]) {
		const dependencies = runner();
		const { outputs, exitCode } = await executePackageWorkflow(
			{
				...workflowInputs,
				behavior_test: true,
				behavior_test_failure_lane: lane,
			},
			dependencies,
		);
		const output = {
			api: "api_package_result",
			api_candidate: "api_candidate_result",
			packaged: "packaged_factories_package_result",
			packaged_candidate: "packaged_factories_candidate_result",
			providers: "model_providers_package_result",
		}[lane];
		for (const [name, value] of Object.entries(outputs))
			assert.equal(value, name === output ? "failure" : "success");
		assert.equal(exitCode, lane ? 1 : 0);
		assert.deepEqual(dependencies.calls, []);
	}
});
