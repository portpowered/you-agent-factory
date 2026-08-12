import assert from "node:assert/strict";
import test from "node:test";

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
		workflowBehaviorScenarios.flatMap((scenario) => Object.keys(scenario.outputs)),
	);

	assert.deepEqual([...names].sort(), [
		"api_candidate_result",
		"api_package_result",
		"model_providers_package_result",
		"packaged_factories_candidate_result",
		"packaged_factories_package_result",
	]);
});
