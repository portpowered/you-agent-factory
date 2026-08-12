import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const outputNames = [
	"api_package_result",
	"api_candidate_result",
	"packaged_factories_package_result",
	"packaged_factories_candidate_result",
	"model_providers_package_result",
];

const emptyOutputs = Object.fromEntries(outputNames.map((name) => [name, ""]));

export const workflowBehaviorScenarios = [
	{
		name: "API package only",
		envPrefix: "API_ONLY",
		workflowResults: ["success"],
		outputs: {
			...emptyOutputs,
			api_package_result: "success",
			api_candidate_result: "success",
		},
	},
	{
		name: "mixed package families",
		envPrefix: "MIXED",
		workflowResults: ["success"],
		outputs: {
			...emptyOutputs,
			api_package_result: "success",
			api_candidate_result: "success",
			packaged_factories_package_result: "success",
			packaged_factories_candidate_result: "success",
			model_providers_package_result: "success",
		},
	},
	{
		name: "no package impact",
		envPrefix: "NO_PACKAGE",
		workflowResults: ["success", "skipped"],
		outputs: emptyOutputs,
	},
	{
		name: "protected-main verification",
		envPrefix: "PROTECTED_MAIN",
		workflowResults: ["success"],
		outputs: {
			...emptyOutputs,
			api_package_result: "success",
			packaged_factories_package_result: "success",
			model_providers_package_result: "success",
		},
	},
	{
		name: "selected package failure",
		envPrefix: "SELECTED_FAILURE",
		workflowResults: ["failure"],
		outputs: {
			...emptyOutputs,
			api_package_result: "failure",
			api_candidate_result: "success",
		},
	},
];

function environmentName(prefix, suffix) {
	return `${prefix}_${suffix}`;
}

function environmentValue(environment, name) {
	return String(environment[name] ?? "").trim();
}

export function observationFromEnvironment(environment, envPrefix) {
	return {
		workflowResult: environmentValue(
			environment,
			environmentName(envPrefix, "WORKFLOW_RESULT"),
		),
		outputs: Object.fromEntries(
			outputNames.map((name) => [
				name,
				environmentValue(
					environment,
					environmentName(envPrefix, name.toUpperCase()),
				),
			]),
		),
	};
}

export function assertWorkflowBehaviorScenario(scenario, observation) {
	assert.ok(
		scenario.workflowResults.includes(observation.workflowResult),
		`${scenario.name} returned ${observation.workflowResult || "missing"}; expected one of ${scenario.workflowResults.join(", ")}`,
	);
	for (const outputName of outputNames) {
		assert.equal(
			observation.outputs[outputName] ?? "",
			scenario.outputs[outputName] ?? "",
			`${scenario.name} output ${outputName}`,
		);
	}
}

export function assertWorkflowBehaviorEnvironment(environment = process.env) {
	const includeFailure = environmentValue(environment, "INCLUDE_FAILURE") === "true";
	for (const scenario of workflowBehaviorScenarios) {
		if (scenario.name === "selected package failure" && !includeFailure) {
			assertWorkflowBehaviorScenario(
				{ ...scenario, workflowResults: ["skipped"], outputs: emptyOutputs },
				{ workflowResult: "skipped", outputs: emptyOutputs },
			);
			continue;
		}
		assertWorkflowBehaviorScenario(
			scenario,
			observationFromEnvironment(environment, scenario.envPrefix),
		);
	}
}

if (
	process.argv[1] &&
	resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))
) {
	assertWorkflowBehaviorEnvironment();
	console.log("Development Package workflow behavior passed.");
}
