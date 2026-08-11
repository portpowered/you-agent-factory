import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { parse } = require("../ui/node_modules/yaml");
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function readWorkflow(relativePath) {
	return parse(readFileSync(join(repositoryRoot, relativePath), "utf8"));
}

function selectedPackageJobs({
	isCiCall,
	runCandidates,
	runApiPackage,
	runPackagedFactoriesPackage,
	runModelProvidersPackage,
}) {
	if (!isCiCall) return [];
	const jobs = [];
	if (runApiPackage) jobs.push("verify_api_package");
	if (runPackagedFactoriesPackage) {
		jobs.push("verify_packaged_factories_package");
	}
	if (runModelProvidersPackage) jobs.push("verify_model_providers_package");
	if (runCandidates && runApiPackage) jobs.push("build_api_candidate");
	if (runCandidates && runPackagedFactoriesPackage) {
		jobs.push("build_packaged_factories_candidate");
	}
	return jobs;
}

const workflow = readWorkflow(".github/workflows/development-package.yml");
const ci = readWorkflow(".github/workflows/ci.yml");

test("Development Package exposes the classifier decision as a reusable contract", () => {
	const inputs = workflow.on.workflow_call.inputs;
	for (const name of [
		"is_ci_call",
		"run_candidates",
		"run_api_package",
		"run_packaged_factories_package",
		"run_model_providers_package",
		"source_commit",
		"pull_request_head_sha",
		"source_ref",
		"repository",
	]) {
		assert.equal(inputs[name].required, true, `${name} must be required`);
	}
	for (const name of [
		"is_ci_call",
		"run_candidates",
		"run_api_package",
		"run_packaged_factories_package",
		"run_model_providers_package",
	]) {
		assert.equal(inputs[name].type, "boolean", `${name} must be boolean`);
	}
	for (const name of [
		"source_commit",
		"pull_request_head_sha",
		"source_ref",
		"repository",
	]) {
		assert.equal(inputs[name].type, "string", `${name} must be string`);
	}
	for (const [job, lane] of [
		["verify_api_package", "run_api_package"],
		["verify_packaged_factories_package", "run_packaged_factories_package"],
		["verify_model_providers_package", "run_model_providers_package"],
	]) {
		assert.match(workflow.jobs[job].if, /inputs\.is_ci_call/);
		assert.match(workflow.jobs[job].if, new RegExp(`inputs\\.${lane}`));
	}
	for (const [job, lane] of [
		["build_api_candidate", "run_api_package"],
		["build_packaged_factories_candidate", "run_packaged_factories_package"],
	]) {
		assert.match(workflow.jobs[job].if, /inputs\.run_candidates/);
		assert.match(workflow.jobs[job].if, new RegExp(`inputs\\.${lane}`));
	}
});

test("package-only, mixed-package, and no-package decisions select exact reusable jobs", () => {
	const cases = [
		{
			name: "API package only",
			input: {
				isCiCall: true,
				runCandidates: true,
				runApiPackage: true,
				runPackagedFactoriesPackage: false,
				runModelProvidersPackage: false,
			},
			jobs: ["verify_api_package", "build_api_candidate"],
		},
		{
			name: "mixed package families",
			input: {
				isCiCall: true,
				runCandidates: true,
				runApiPackage: true,
				runPackagedFactoriesPackage: true,
				runModelProvidersPackage: true,
			},
			jobs: [
				"verify_api_package",
				"verify_packaged_factories_package",
				"verify_model_providers_package",
				"build_api_candidate",
				"build_packaged_factories_candidate",
			],
		},
		{
			name: "no package impact",
			input: {
				isCiCall: true,
				runCandidates: true,
				runApiPackage: false,
				runPackagedFactoriesPackage: false,
				runModelProvidersPackage: false,
			},
			jobs: [],
		},
	];

	for (const { name, input, jobs } of cases) {
		assert.deepEqual(selectedPackageJobs(input), jobs, name);
	}
});

test("candidate work is disabled for protected-main CI while direct publication remains", () => {
	const selected = selectedPackageJobs({
		isCiCall: true,
		runCandidates: false,
		runApiPackage: true,
		runPackagedFactoriesPackage: true,
		runModelProvidersPackage: true,
	});
	assert.deepEqual(selected, [
		"verify_api_package",
		"verify_packaged_factories_package",
		"verify_model_providers_package",
	]);
	assert.match(
		workflow.jobs["prepare-main-candidate"].if,
		/inputs\.is_ci_call != true/,
	);
	assert.match(
		workflow.jobs["publish-main-candidate"].if,
		/inputs\.is_ci_call != true/,
	);
	assert.match(workflow.concurrency.group, /inputs\.is_ci_call == true/);
	assert.match(
		workflow.concurrency["cancel-in-progress"],
		/inputs\.is_ci_call != true/,
	);
});

test("main CI passes classifier decisions and package failures to Verification Policy", () => {
	const caller = ci.jobs["development-package"];
	assert.equal(caller.uses, "./.github/workflows/development-package.yml");
	assert.match(
		caller.with.run_api_package,
		/needs\.classify\.outputs\.run_api_package != 'false'/,
	);
	assert.match(
		caller.with.run_packaged_factories_package,
		/needs\.classify\.outputs\.run_packaged_factories_package != 'false'/,
	);
	assert.match(
		caller.with.run_model_providers_package,
		/needs\.classify\.outputs\.run_model_providers_package != 'false'/,
	);
	assert.equal(caller.with.source_commit, "${{ github.event.pull_request.head.sha || github.sha }}");

	const policy = ci.jobs["verification-policy"];
	assert.ok(policy.needs.includes("development-package"));
	assert.match(
		policy.steps[0].env.RESULTS,
		/package_workflow=\$\{\{ needs\.development-package\.result \}\}/,
	);
	assert.match(
		policy.steps[0].env.RESULTS,
		/api_candidate=\$\{\{ needs\.development-package\.outputs\.api_candidate_result \}\}/,
	);
	assert.equal(
		workflow.on.workflow_call.outputs.api_package_result.value,
		"${{ jobs.verify_api_package.outputs.result }}",
	);
	assert.equal(
		workflow.on.workflow_call.outputs.api_candidate_result.value,
		"${{ jobs.build_api_candidate.outputs.result }}",
	);
});
