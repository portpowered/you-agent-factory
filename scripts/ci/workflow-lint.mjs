import { spawnSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { extname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { frontendPlan, apiPlan, workflowPlan } from "./verification-plans.mjs";

const WORKFLOW_EXTENSIONS = new Set([".yml", ".yaml"]);

function comparePaths(left, right) {
	if (left < right) return -1;
	if (left > right) return 1;
	return 0;
}

export function discoverWorkflowFiles(workflowDirectory = ".github/workflows") {
	const directory = resolve(workflowDirectory);
	return readdirSync(directory, { withFileTypes: true })
		.filter((entry) => entry.isFile() && WORKFLOW_EXTENSIONS.has(extname(entry.name).toLowerCase()))
		.map((entry) => join(directory, entry.name))
		.sort(comparePaths);
}

function workflowJobSection(workflow, jobName) {
	const match = workflow.match(
		new RegExp(`\\n  ${jobName}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9_-]+:\\n|\\s*$)`),
	);
	if (!match) throw new Error(`workflow contract is missing job: ${jobName}`);
	return match[0];
}

export function validateTaggedPackageWorkflowContract({ workflow } = {}) {
	const job = workflowJobSection(workflow, "publish-tagged-release");
	requireWorkflowText(
		workflow,
		"BUN_VERSION: 1.3.12",
		"tagged Bun must remain pinned",
	);
	requireWorkflowText(
		job,
		"needs: resolve-release-tag",
		"tagged preparation and publication share one job",
	);
	requireWorkflowText(
		job,
		"environment: development-publishing",
		"tagged publishing environment is unchanged",
	);
	requireWorkflowText(
		job,
		"id-token: write",
		"tagged provenance retains OIDC permission",
	);
	requireWorkflowText(
		job,
		"ref: ${{ github.event.workflow_run.head_sha }}",
		"tagged checkout retains verified source",
	);
	requireWorkflowText(
		job,
		'--expected-source-commit "${{ github.event.workflow_run.head_sha }}"',
		"publisher validates source identity",
	);
	requireWorkflowOrder(
		job,
		"bun run --bun scripts/public-release-package-candidate.mjs",
		"bun run --bun scripts/public-release-package-publish.mjs",
		"tagged candidate is prepared before publication",
	);
	if (
		/\n  prepare-public-package-candidate:|actions\/(?:upload|download)-artifact|npm install/.test(
			job,
		) ||
		/\n  prepare-public-package-candidate:/.test(workflow)
	) {
		throw new Error(
			"tagged preparation must not have an artifact hop or repeated setup",
		);
	}
	return { name: "tagged-package-workflow", status: "pass" };
}

function requireWorkflowMatch(value, pattern, description) {
	if (!pattern.test(value)) throw new Error(`workflow contract failed: ${description}`);
}

export function validateReusablePackageWorkflowContract({ workflow } = {}) {
	const job = workflowJobSection(workflow, "verify_api_package");
	const results = ["api_package_result", "api_candidate_result", "packaged_factories_package_result", "packaged_factories_candidate_result", "model_providers_package_result"];
	for (const name of results) {
		requireWorkflowText(workflow, `value: \${{ jobs.verify_api_package.outputs.${name} }}`, "retain all five reusable outputs");
		requireWorkflowText(job, `steps.record_result.outputs.${name}`, "retain independent results");
	}
	requireWorkflowText(job, "inputs.is_ci_call && (inputs.run_api_package || inputs.run_packaged_factories_package || inputs.run_model_providers_package)", "retain selection");
	requireWorkflowText(job, "ref: ${{ inputs.source_commit }}", "check out requested source");
	requireWorkflowText(job, "bun-version: ${{ env.BUN_VERSION }}", "pin shared Bun setup");
	requireWorkflowText(job, "PACKAGE_WORKFLOW_INPUTS: ${{ toJSON(inputs) }}", "preserve typed inputs/defaults");
	requireWorkflowText(job, "bun run --bun scripts/public-package-workflow.mjs", "single read-only artifact runner");
	if (/\n  (?:build_api_candidate|build_packaged_factories_candidate|verify_packaged_factories_package|verify_model_providers_package):/.test(workflow) || /id-token:|contents: write|npm |actions\/(?:upload|download)-artifact|continue-on-error/.test(job)) {
		throw new Error("reusable package verification must remain one read-only job without artifact hops");
	}
	return { name: "reusable-package-workflow", status: "pass" };
}

function requireWorkflowText(value, text, description) {
	if (!value.includes(text)) throw new Error(`workflow contract failed: ${description}`);
}

function workflowStepSection(job, stepName) {
	const marker = `      - name: ${stepName}\n`;
	const start = job.indexOf(marker);
	if (start < 0) throw new Error(`workflow contract is missing step: ${stepName}`);
	const end = job.indexOf("\n      - name:", start + marker.length);
	return job.slice(start, end < 0 ? undefined : end);
}

function requireWorkflowOrder(value, first, second, description) {
	if (value.indexOf(first) < 0 || value.indexOf(second) < 0 || value.indexOf(first) >= value.indexOf(second)) {
		throw new Error(`workflow contract failed: ${description}`);
	}
}

// CI topology belongs in Workflow Lint, rather than a product runtime test.
export function validateFrontendSharedSetupWorkflowContract({ workflow } = {}) {
	const frontend = workflowJobSection(workflow, "frontend");
	const browser = workflowJobSection(workflow, "frontend-browser");
	const policy = workflowJobSection(workflow, "verification-policy");
	requireWorkflowText(workflow, "BUN_VERSION: 1.3.12", "frontend Bun must remain pinned");
	requireWorkflowText(frontend, "bun-version: ${{ env.BUN_VERSION }}", "use the pinned Bun version");
	requireWorkflowText(frontend, "path: ~/.bun/install/cache", "cache Bun downloads only");
	requireWorkflowText(frontend,
		"key: frontend-bun-v1-${{ runner.os }}-${{ runner.arch }}-${{ env.BUN_VERSION }}-${{ hashFiles('ui/bun.lock') }}",
		"cache identity must include platform, Bun and frozen lock");
	requireWorkflowText(frontend, "run: cd ui && bun install --frozen-lockfile", "install frozen dependencies");
	requireWorkflowText(frontend, "run: make typecheck", "retain typecheck");
	for (const [name, command] of [["Lint frontend", "make ui-lint"],
		["Run frontend unit and replay coverage", "make test-ui-coverage"]]) {
		const step = workflowStepSection(frontend, name);
		requireWorkflowText(step, "if: success() || failure()", "attempt later proof after failures, but not cancellation");
		requireWorkflowText(step, `run: ${command}`, "retain the complete proof command");
	}
	requireWorkflowText(frontend, 'UI_COVERAGE_MAIN_MAX_WORKERS: "4"', "retain coverage worker budget");
	requireWorkflowOrder(frontend, "bun install --frozen-lockfile", "make typecheck", "install before static proof");
	requireWorkflowOrder(frontend, "make typecheck", "make ui-lint", "retain static order");
	requireWorkflowOrder(frontend, "make ui-lint", "make test-ui-coverage", "coverage shares static setup");
	if (/setup-go|continue-on-error/.test(frontend) || /\n  ui-coverage:/.test(workflow)) {
		throw new Error("frontend shared setup must not duplicate setup or mask failures");
	}
	requireWorkflowMatch(policy, /needs: \[[^\]]*\bfrontend\b[^\]]*\]/, "policy needs shared Frontend");
	for (const result of ["FRONTEND_RESULT", "FRONTEND_COVERAGE_RESULT"]) {
		requireWorkflowText(policy, `${result}: \${{ needs.frontend.result }}`, "static and coverage share the aggregate result");
	}
	requireWorkflowText(policy, "FRONTEND_COMPONENT_RESULT: ${{ needs.frontend.result }}", "Component shares aggregate result");
	for (const result of ["FRONTEND_BROWSER_RESULT", "FRONTEND_STORYBOOK_RESULT"]) {
		requireWorkflowText(policy, `${result}: \${{ needs.frontend-browser.result }}`, "browser proofs share their own aggregate result");
	}
	requireWorkflowMatch(policy, /needs: \[[^\]]*\bfrontend-browser\b[^\]]*\]/, "policy needs Frontend Browser");
	for (const [job, suite] of [[frontend, "component"], [browser, "browser"]]) {
		requireWorkflowText(job, "needs: classify", "both frontend jobs retain selection");
		requireWorkflowText(job, "if: always() && needs.classify.outputs.run_frontend != 'false'", "both frontend jobs retain selection");
		requireWorkflowText(job, "bun-version: ${{ env.BUN_VERSION }}", "use the pinned Bun version");
		requireWorkflowText(job, "path: ~/.bun/install/cache", "cache Bun downloads only");
		requireWorkflowText(job, "key: frontend-bun-v1-${{ runner.os }}-${{ runner.arch }}-${{ env.BUN_VERSION }}-${{ hashFiles('ui/bun.lock') }}", "cache identity must include platform, Bun and frozen lock");
		if (job.split("bun install --frozen-lockfile").length !== 2) throw new Error("workflow contract requires one frozen install per frontend job");
		requireWorkflowText(job, "run: bash scripts/ci/run-frontend-verification.sh", "run retained frontend proofs");
		requireWorkflowText(job, `FRONTEND_SUITE: ${suite}`, "complete suites use separate runners");
	}
	requireWorkflowText(browser, "path: ~/.cache/ms-playwright", "retain browser cache");
	requireWorkflowText(browser, "key: playwright-chromium-v1-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('ui/bun.lock') }}", "browser cache follows platform and lock");
	requireWorkflowText(browser, "run: make ui-install-playwright", "install browsers on cache miss");
	if (/\n  frontend-(component|storybook):/.test(workflow)) throw new Error("workflow contract duplicates frontend jobs");
	if (policy.includes("ui-coverage")) throw new Error("policy must not depend on removed coverage job");
	return { name: "frontend-shared-setup-workflow", status: "pass" };
}

export function validateConsolidatedCIWorkflowContract({ workflow } = {}) {
	const setup = workflowJobSection(workflow, "classify");
	const api = workflowJobSection(workflow, "api-pr-verification");
	const packages = workflowJobSection(workflow, "development-package");
	const policy = workflowJobSection(workflow, "verification-policy");
	for (const text of ["name: Verification Setup", "actionlint@v1.7.12", "run: bash scripts/ci/run-workflow-verification.sh",
		"docs_result: ${{ steps.docs-reference.outcome }}", "if: (success() || failure()) && steps.classify.outputs.run_docs_reference != 'false'", "run: make docs-reference-smoke"]) {
		requireWorkflowText(setup, text, "retain shared setup proof and fail-closed selection");
	}
	requireWorkflowText(packages, "run_api_package: ${{ github.event_name != 'pull_request' && needs.classify.outputs.run_api_package != 'false' }}", "retain non-PR API selection");
	requireWorkflowText(api, "if: always() && github.event_name == 'pull_request' && needs.classify.outputs.run_api_package != 'false'", "independent selected PR API proof");
	requireWorkflowText(api, "run: bash scripts/ci/run-api-pr-verification.sh", "retain API proofs");
	for (const [result, expression] of [["DOCS_RESULT", "needs.classify.outputs.docs_result"], ["WORKFLOW_LINT_RESULT", "needs.classify.result"],
		["API_RESULT", "github.event_name == 'pull_request' && needs.api-pr-verification.result || needs.development-package.outputs.api_package_result"],
		["API_CANDIDATE_RESULT", "github.event_name == 'pull_request' && needs.api-pr-verification.result || needs.development-package.outputs.api_candidate_result"]]) {
		requireWorkflowText(policy, `${result}: \${{ ${expression} }}`, "map merged required proof");
	}
	requireWorkflowText(policy, "API_INDEPENDENT: ${{ github.event_name == 'pull_request' }}", "API-only PR does not require reusable children");
	if (/\n  (workflow-lint|docs-reference):/.test(workflow)) throw new Error("workflow contract duplicates cheap jobs");
	const targets = frontendPlan().map((step) => step.args[0]);
	if (JSON.stringify(targets) !== JSON.stringify(["ui-component-test", "ui-integration-test", "ui-storybook-integration-test"])) {
		throw new Error("workflow contract must retain every frontend suite and build once");
	}
	const apiTargets = apiPlan({}).slice(0, 4).map((step) => step.args[0]);
	if (JSON.stringify(apiTargets) !== JSON.stringify(["ui-deps", "contracts-smoke", "api-smoke", "api-package-verify"])) throw new Error("workflow contract must retain API proof commands");
	if (!workflowPlan().some((step) => step.args.includes("scripts/ci/functional-compile-cache.test.py"))) throw new Error("workflow contract must retain compiler-cache proof");
	return { name: "consolidated-ci-workflow", status: "pass" };
}

/**
 * Enforce the TTS integration wiring as a static workflow contract.
 *
 * This is intentionally part of the executable Workflow Lint gate. The
 * contract protects CI composition (helper handoff, platform execution, and
 * verification-policy aggregation), not product runtime behavior.
 */
export function validateTtsCleanInstallWorkflowContract({ workflow, makefile } = {}) {
	if (typeof workflow !== "string" || typeof makefile !== "string") {
		throw new Error("workflow contract requires workflow and Makefile text");
	}

	const integrationJob = workflowJobSection(workflow, "backend-integration");
	const windowsJob = workflowJobSection(workflow, "tts-clean-install-windows");
	const policyJob = workflowJobSection(workflow, "verification-policy");

	requireWorkflowMatch(
		makefile,
		/test-integration:[\s\S]*\.\/tests\/integration\/models\/tts_clean_install/,
		"test-integration must execute the TTS clean-install package",
	);
	requireWorkflowMatch(
		integrationJob,
		/name: Build TTS clean-install helper artifact/,
		"Backend Integration must build the TTS helper artifact",
	);
	requireWorkflowMatch(
		integrationJob,
		/INFINITE_YOU_TTS_PREBUILT_HELPER_PATH/,
		"Backend Integration must export the prebuilt TTS helper path",
	);
	requireWorkflowMatch(
		windowsJob,
		/runs-on: windows-latest/,
		"TTS clean-install execution must use the Windows boundary",
	);
	requireWorkflowMatch(
		windowsJob,
		/go test -c -o \$helperPath \.\/tests\/integration\/models\/tts_clean_install/,
		"the Windows job must compile the TTS helper once",
	);
	requireWorkflowMatch(
		windowsJob,
		/go test \.\/tests\/integration\/models\/tts_clean_install -count=1 -v -timeout=20m/,
		"the Windows job must execute the TTS package",
	);
	requireWorkflowMatch(
		windowsJob,
		/INFINITE_YOU_TTS_PREBUILT_HELPER_SHA256/,
		"the Windows job must pass the helper identity",
	);
	requireWorkflowMatch(
		policyJob,
		/needs: \[[^\]]*\btts-clean-install-windows\b[^\]]*\]/s,
		"Verification Policy must depend on the Windows TTS job",
	);
	requireWorkflowMatch(
		policyJob,
		/needs\.tts-clean-install-windows\.result/,
		"Verification Policy must aggregate the Windows TTS result",
	);

	return { name: "tts-clean-install-workflow", status: "pass" };
}

/**
 * Keep bounded raw failure evidence in the always-run functional artifact.
 * This contract protects the diagnostic output reviewers download from CI.
 */
export function validateFunctionalDiagnosticsArtifactWorkflowContract({ workflow } = {}) {
	if (typeof workflow !== "string") {
		throw new Error("functional diagnostics workflow contract requires workflow text");
	}

	const functionalJob = workflowJobSection(workflow, "backend-coverage");
	const verdictMarker = "      - name: Report functional coverage verdict";
	const uploadMarker = "      - name: Upload functional test diagnostics";
	const verdictOffset = functionalJob.indexOf(verdictMarker);
	const uploadOffset = functionalJob.indexOf(uploadMarker);
	if (verdictOffset < 0) {
		throw new Error("workflow contract is missing the functional coverage verdict step");
	}
	if (uploadOffset < 0) {
		throw new Error("workflow contract is missing the functional diagnostics upload step");
	}
	if (uploadOffset <= verdictOffset) {
		throw new Error("workflow contract requires functional diagnostics upload after the coverage verdict");
	}

	const nextStepOffset = functionalJob.indexOf("\n      - name:", uploadOffset + uploadMarker.length);
	const uploadStep = functionalJob.slice(uploadOffset, nextStepOffset < 0 ? undefined : nextStepOffset);
	requireWorkflowMatch(
		uploadStep,
		/^        if: always\(\) && matrix\.suite == 'functional'\s*$/m,
		"functional diagnostics upload must run after failures and only for the functional matrix row",
	);
	requireWorkflowMatch(
		uploadStep,
		/^        uses: actions\/upload-artifact@v4\s*$/m,
		"functional diagnostics must use the pinned artifact action",
	);
	requireWorkflowMatch(
		uploadStep,
		/^          name: functional-test-diagnostics\s*$/m,
		"raw evidence must join the existing functional diagnostics artifact",
	);
	requireWorkflowMatch(
		uploadStep,
		/^            \.artifacts\/functional-test-viz\/raw-failures\/index\.json\s*$/m,
		"functional diagnostics artifact must include the raw failure index",
	);
	requireWorkflowMatch(
		uploadStep,
		/^            \.artifacts\/functional-test-viz\/raw-failures\/\*\.jsonl\s*$/m,
		"functional diagnostics artifact must include package-keyed raw failure files",
	);
	requireWorkflowMatch(
		uploadStep,
		/^          if-no-files-found: ignore\s*$/m,
		"an all-green run must not fail when raw files are absent",
	);
	requireWorkflowMatch(
		uploadStep,
		/^          retention-days: 14\s*$/m,
		"functional diagnostic retention must remain 14 days",
	);

	return { name: "functional-diagnostics-artifact-workflow", status: "pass" };
}

export function validateBackendLintWorkflowContract({ workflow, makefile }) {
	const job = workflowJobSection(workflow, "backend-lint");
	if (/go test[^\n]*-race/.test(job)) throw new Error("Backend Lint must not run a race step");
	for (const duplicate of ["Exercise packaged Markdown enforcement", "Check Go formatting", "Build and smoke-test shared lint plugin"]) {
		if (job.includes(`- name: ${duplicate}`)) throw new Error(`Backend Lint repeats enforcement: ${duplicate}`);
	}
	const runs = job.match(/make LINT_REPORT_FILE="\$LINT_REPORT_FILE" lint/g) ?? [];
	if (runs.length !== 1) throw new Error("Backend Lint requires one complete canonical inventory run");
	requireWorkflowText(job, "scripts/lint-migration-smoke.py ci-smoke", "real plugin diagnostic smoke");
	requireWorkflowText(job, "scripts/build-golangci.py --restore", "validated artifact restore");
	requireWorkflowText(job, 'GOLANGCI_PREBUILT: "1"', "canonical prebuilt plugin reuse");
	requireWorkflowText(makefile, "LINT_TARGETS_BASE := vet model-provider-package-check golangci docs-reference-check fmt-check contracts-check", "complete base enforcement inventory");
	requireWorkflowText(makefile, "golangci-lint-run: golangci-build", "built-in checks use the prepared custom binary");
	return { name: "backend-lint-workflow", status: "pass" };
}

export function validateRepositoryWorkflowContracts({ repositoryRoot = process.cwd() } = {}) {
	const root = resolve(repositoryRoot);
	const workflow = readFileSync(join(root, ".github", "workflows", "ci.yml"), "utf8");
	return {
		contracts: [
			validateReusablePackageWorkflowContract({
				workflow: readFileSync(join(root, ".github", "workflows", "development-package.yml"), "utf8"),
			}),
			validateTaggedPackageWorkflowContract({
				workflow: readFileSync(join(root, ".github", "workflows", "development-package.yml"), "utf8"),
			}),
			validateBackendLintWorkflowContract({
				workflow,
				makefile: readFileSync(join(root, "Makefile"), "utf8"),
			}),
			validateTtsCleanInstallWorkflowContract({
				workflow,
				makefile: readFileSync(join(root, "Makefile"), "utf8"),
			}),
			validateFunctionalDiagnosticsArtifactWorkflowContract({ workflow }),
		],
	};
}

export function runWorkflowLint({
	actionlint = process.env.ACTIONLINT_BIN || "actionlint",
	workflowDirectory = ".github/workflows",
	spawn = spawnSync,
	log = console.log,
	validateRepositoryContracts = false,
} = {}) {
	const workflowFiles = discoverWorkflowFiles(workflowDirectory);
	if (workflowFiles.length === 0) {
		throw new Error(`No GitHub Actions workflow files found in ${resolve(workflowDirectory)}.`);
	}

	log(`WORKFLOW_LINT_FILE_COUNT=${workflowFiles.length}`);
	const result = spawn(actionlint, workflowFiles, {
		stdio: "inherit",
		windowsHide: true,
	});
	if (result.error) {
		throw new Error(`Unable to execute ${actionlint}: ${result.error.message}`);
	}
	if (result.status !== 0) {
		const termination = result.signal ? ` after signal ${result.signal}` : "";
		throw new Error(`Workflow schema lint failed with exit code ${result.status}${termination}.`);
	}
	if (validateRepositoryContracts) {
		validateConsolidatedCIWorkflowContract({
			workflow: readFileSync(join(process.cwd(), ".github", "workflows", "ci.yml"), "utf8"),
		});
		validateFrontendSharedSetupWorkflowContract({
			workflow: readFileSync(join(process.cwd(), ".github", "workflows", "ci.yml"), "utf8"),
		});
		validateRepositoryWorkflowContracts();
		log("WORKFLOW_LINT_STATIC_CONTRACTS_OK");
	}

	log(`WORKFLOW_LINT_OK files=${workflowFiles.length}`);
	return { actionlint, workflowFiles, status: result.status };
}

function parseArguments(args) {
	const options = {};
	for (let index = 0; index < args.length; index += 1) {
		const argument = args[index];
		if (argument === "--actionlint") {
			options.actionlint = args[++index];
			if (!options.actionlint) throw new Error("--actionlint requires a path or command");
			continue;
		}
		if (argument === "--workflow-directory") {
			options.workflowDirectory = args[++index];
			if (!options.workflowDirectory) throw new Error("--workflow-directory requires a path");
			continue;
		}
		throw new Error(`Unknown argument: ${argument}`);
	}
	return options;
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	try {
		runWorkflowLint({ ...parseArguments(process.argv.slice(2)), validateRepositoryContracts: true });
	} catch (error) {
		console.error(`workflow-lint: ${error.message}`);
		process.exitCode = 1;
	}
}
