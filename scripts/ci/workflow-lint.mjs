import { spawnSync } from "node:child_process";
import { readdirSync } from "node:fs";
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

export function runWorkflowLint({
	actionlint = process.env.ACTIONLINT_BIN || "actionlint",
	workflowDirectory = ".github/workflows",
	spawn = spawnSync,
	log = console.log,
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
		runWorkflowLint(parseArguments(process.argv.slice(2)));
	} catch (error) {
		console.error(`workflow-lint: ${error.message}`);
		process.exitCode = 1;
	}
}
