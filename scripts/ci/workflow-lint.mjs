import { spawnSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { extname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

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
		new RegExp(`\\n  ${jobName}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9-]+:\\n|\\s*$)`),
	);
	if (!match) throw new Error(`workflow contract is missing job: ${jobName}`);
	return match[0];
}

function requireWorkflowMatch(value, pattern, description) {
	if (!pattern.test(value)) throw new Error(`workflow contract failed: ${description}`);
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

export function validateRepositoryWorkflowContracts({ repositoryRoot = process.cwd() } = {}) {
	const root = resolve(repositoryRoot);
	return {
		contracts: [
			validateTtsCleanInstallWorkflowContract({
				workflow: readFileSync(join(root, ".github", "workflows", "ci.yml"), "utf8"),
				makefile: readFileSync(join(root, "Makefile"), "utf8"),
			}),
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
