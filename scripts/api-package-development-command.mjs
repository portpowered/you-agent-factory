import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { prepareCandidate } from "./api-package-candidate.mjs";
import {
	assertDevelopmentPackageAction,
	DEVELOPMENT_PACKAGE_ACTIONS,
} from "./api-package-development-policy.mjs";
import { validatePullRequestCandidate } from "./api-package-pr-dry-run.mjs";
import { publishCandidateDirectory } from "./api-package-publish.mjs";

const productionDependencies = Object.freeze({
	prepareCandidate,
	publishCandidateDirectory,
	validatePullRequestCandidate,
});

export async function executeDevelopmentPackageCommand(
	input,
	dependencies = productionDependencies,
) {
	const plan = assertDevelopmentPackageAction(input, input.action);
	switch (input.action) {
		case DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN:
			return dependencies.validatePullRequestCandidate({
				eventName: input.eventName,
				outputDirectory: input.outputDirectory,
				packageDirectory: input.packageDirectory,
				runId: input.runId,
				sourceCommit: plan.sourceCommit,
				workspaceDirectory: input.workspaceDirectory,
			});
		case DEVELOPMENT_PACKAGE_ACTIONS.PREPARE_MAIN:
			return dependencies.prepareCandidate({
				outputDirectory: input.outputDirectory,
				packageDirectory: input.packageDirectory,
				runId: input.runId,
				sourceCommit: plan.sourceCommit,
			});
		case DEVELOPMENT_PACKAGE_ACTIONS.PUBLISH_MAIN:
			return dependencies.publishCandidateDirectory({
				candidateDirectory: input.candidateDirectory,
				expectedSourceCommit: plan.sourceCommit,
				workspaceDirectory: input.workspaceDirectory,
			});
		default:
			throw new Error(
				`[api-package-development-command] unsupported action ${input.action}`,
			);
	}
}

async function main() {
	const { values } = parseArgs({
		options: {
			action: { type: "string" },
			"candidate-directory": { type: "string" },
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
	const result = await executeDevelopmentPackageCommand({
		action: values.action,
		candidateDirectory: values["candidate-directory"],
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
	process.stdout.write(`${JSON.stringify(result.evidence ?? result)}\n`);
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
