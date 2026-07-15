import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

export const DEVELOPMENT_PACKAGE_ACTIONS = Object.freeze({
	DRY_RUN: "dry-run",
	PREPARE_MAIN: "prepare-main",
	PUBLISH_MAIN: "publish-main",
});

export const DEVELOPMENT_PACKAGE_OUTCOMES = Object.freeze({
	BLOCKED: "PREREQUISITES_BLOCKED",
	INELIGIBLE: "EVENT_INELIGIBLE",
	PR_DRY_RUN: "PR_DRY_RUN",
	PROTECTED_MAIN: "PROTECTED_MAIN",
});

const PROTECTED_REPOSITORY = "portpowered/you-agent-factory";
const PROTECTED_MAIN_REF = "refs/heads/main";

function requiredString(value, name) {
	if (typeof value !== "string" || value.length === 0) {
		throw new Error(`[api-package-development-policy] ${name} is required`);
	}
	return value;
}

export function planDevelopmentPackage(context) {
	if (context.prerequisiteResult !== "success") {
		return { outcome: DEVELOPMENT_PACKAGE_OUTCOMES.BLOCKED, allowedActions: [] };
	}

	if (context.eventName === "pull_request") {
		const sourceCommit = requiredString(context.sourceCommit, "source commit");
		const pullRequestHeadSha = requiredString(
			context.pullRequestHeadSha,
			"pull request head SHA",
		);
		if (sourceCommit !== pullRequestHeadSha) {
			throw new Error(
				"[api-package-development-policy] pull request candidate must use the reviewed head SHA",
			);
		}
		return {
			outcome: DEVELOPMENT_PACKAGE_OUTCOMES.PR_DRY_RUN,
			allowedActions: [DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN],
			sourceCommit,
		};
	}

	if (
		context.eventName === "push" &&
		context.ref === PROTECTED_MAIN_REF &&
		context.repository === PROTECTED_REPOSITORY
	) {
		return {
			outcome: DEVELOPMENT_PACKAGE_OUTCOMES.PROTECTED_MAIN,
			allowedActions: [
				DEVELOPMENT_PACKAGE_ACTIONS.PREPARE_MAIN,
				DEVELOPMENT_PACKAGE_ACTIONS.PUBLISH_MAIN,
			],
			sourceCommit: requiredString(context.sourceCommit, "source commit"),
		};
	}

	return { outcome: DEVELOPMENT_PACKAGE_OUTCOMES.INELIGIBLE, allowedActions: [] };
}

export function assertDevelopmentPackageAction(context, expectedAction) {
	const action = requiredString(expectedAction, "expected action");
	const plan = planDevelopmentPackage(context);
	if (!plan.allowedActions.includes(action)) {
		throw new Error(
			`[api-package-development-policy] ${action} is not allowed for outcome ${plan.outcome}`,
		);
	}
	return plan;
}

export async function executeDevelopmentPackagePolicy(input, dependencies) {
	const prerequisiteResult = await dependencies.runPrerequisites();
	const plan = planDevelopmentPackage({ ...input, prerequisiteResult });
	if (
		plan.outcome === DEVELOPMENT_PACKAGE_OUTCOMES.BLOCKED ||
		plan.outcome === DEVELOPMENT_PACKAGE_OUTCOMES.INELIGIBLE
	) {
		return plan;
	}

	if (plan.outcome === DEVELOPMENT_PACKAGE_OUTCOMES.PR_DRY_RUN) {
		return dependencies.validatePullRequestCandidate({
			eventName: input.eventName,
			outputDirectory: input.outputDirectory,
			packageDirectory: input.packageDirectory,
			runId: input.runId,
			sourceCommit: plan.sourceCommit,
			workspaceDirectory: input.workspaceDirectory,
		});
	}

	const candidate = await dependencies.prepareCandidate({
		outputDirectory: input.outputDirectory,
		packageDirectory: input.packageDirectory,
		runId: input.runId,
		sourceCommit: plan.sourceCommit,
	});
	return dependencies.publishAndVerifyCandidate({
		...candidate,
		workspaceDirectory: input.workspaceDirectory,
	});
}

async function main() {
	const { values } = parseArgs({
		options: {
			"event-name": { type: "string" },
			"expected-action": { type: "string" },
			"prerequisite-result": { type: "string" },
			"pull-request-head-sha": { type: "string" },
			ref: { type: "string" },
			repository: { type: "string" },
			"source-commit": { type: "string" },
		},
		strict: true,
	});
	const plan = assertDevelopmentPackageAction(
		{
			eventName: values["event-name"],
			prerequisiteResult: values["prerequisite-result"],
			pullRequestHeadSha: values["pull-request-head-sha"],
			ref: values.ref,
			repository: values.repository,
			sourceCommit: values["source-commit"],
		},
		values["expected-action"],
	);
	process.stdout.write(`${JSON.stringify(plan)}\n`);
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
