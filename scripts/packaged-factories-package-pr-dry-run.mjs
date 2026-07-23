import { writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import {
	assertDevelopmentPackageAction,
	DEVELOPMENT_PACKAGE_ACTIONS,
} from "./package-development-policy.mjs";
import { prepareCandidate } from "./packaged-factories-package-candidate.mjs";
import { installAndVerifyTarball } from "./packaged-factories-package-consumer.mjs";

export const DRY_RUN_OUTCOME = "DRY_RUN_NO_PUBLISH";

const DIAGNOSTIC_PREFIX = "[packaged-factories-package-pr-dry-run]";

function requiredString(value, name) {
	if (typeof value !== "string" || value.length === 0) {
		throw new Error(`${DIAGNOSTIC_PREFIX} ${name} is required`);
	}
	return value;
}

function preparationStage(error) {
	const message = error instanceof Error ? error.message : String(error);
	if (message.includes("generated catalog drift check")) {
		return "generation";
	}
	if (message.includes("inventory")) {
		return "inventory";
	}
	if (message.startsWith("[package-release-candidate]")) {
		return "identity";
	}
	if (message.startsWith("[packaged-factories-package-pack]")) {
		return "pack";
	}
	return "candidate";
}

function stageError(stage, error) {
	const message = error instanceof Error ? error.message : String(error);
	return new Error(`${DIAGNOSTIC_PREFIX} ${stage} stage failed: ${message}`, {
		cause: error,
	});
}

export async function validatePullRequestCandidate(input, dependencies = {}) {
	const plan = assertDevelopmentPackageAction(
		input,
		DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN,
	);
	const outputDirectory = resolve(
		requiredString(input.outputDirectory, "output directory"),
	);
	const prepare = dependencies.prepareCandidate ?? prepareCandidate;
	const verify =
		dependencies.installAndVerifyTarball ?? installAndVerifyTarball;

	let candidate;
	try {
		candidate = await prepare({
			packageDirectory: requiredString(
				input.packageDirectory,
				"package directory",
			),
			outputDirectory,
			runId: requiredString(input.runId, "run ID"),
			sourceCommit: plan.sourceCommit,
		});
	} catch (error) {
		throw stageError(preparationStage(error), error);
	}

	let consumerEvidence;
	try {
		consumerEvidence = await verify({
			expectedSourceCommit: plan.sourceCommit,
			expectedVersion: candidate.evidence.candidateVersion,
			packageName: candidate.evidence.packageName,
			tarballPath: candidate.tarballPath,
			workspaceDirectory: requiredString(
				input.workspaceDirectory,
				"workspace directory",
			),
		});
	} catch (error) {
		throw stageError("installed-consumer", error);
	}

	const consumerEvidencePath = join(outputDirectory, "consumer-evidence.json");
	await writeFile(
		consumerEvidencePath,
		`${JSON.stringify(consumerEvidence, null, 2)}\n`,
	);
	return {
		...candidate.evidence,
		consumerEvidenceFile: "consumer-evidence.json",
		outcome: DRY_RUN_OUTCOME,
	};
}

async function main() {
	const { values } = parseArgs({
		options: {
			"event-name": { type: "string" },
			"output-directory": { type: "string" },
			"package-directory": { type: "string" },
			"prerequisite-result": { type: "string" },
			"pull-request-head-sha": { type: "string" },
			ref: { type: "string" },
			repository: { type: "string" },
			"run-id": { type: "string" },
			"source-commit": { type: "string" },
			"workspace-directory": { type: "string" },
		},
		strict: true,
	});
	const result = await validatePullRequestCandidate({
		eventName: values["event-name"],
		outputDirectory: values["output-directory"],
		packageDirectory: values["package-directory"],
		prerequisiteResult: values["prerequisite-result"],
		pullRequestHeadSha: values["pull-request-head-sha"],
		ref: values.ref,
		repository: values.repository,
		runId: values["run-id"],
		sourceCommit: values["source-commit"],
		workspaceDirectory: values["workspace-directory"],
	});
	process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (
	process.argv[1] &&
	import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
	main().catch((error) => {
		process.stderr.write(`${error.message}\n`);
		process.exitCode = 1;
	});
}
