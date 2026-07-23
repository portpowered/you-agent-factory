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

const DIAGNOSTIC_PREFIX = "[package-development-policy]";
const PROTECTED_REPOSITORY = "portpowered/you-agent-factory";
const PROTECTED_MAIN_REF = "refs/heads/main";

function requiredString(value, name) {
	if (typeof value !== "string" || value.length === 0) {
		throw new Error(`${DIAGNOSTIC_PREFIX} ${name} is required`);
	}
	return value;
}

export function planDevelopmentPackage(context) {
	if (context.prerequisiteResult !== "success") {
		return {
			outcome: DEVELOPMENT_PACKAGE_OUTCOMES.BLOCKED,
			allowedActions: [],
		};
	}

	if (context.eventName === "pull_request") {
		const sourceCommit = requiredString(context.sourceCommit, "source commit");
		const pullRequestHeadSha = requiredString(
			context.pullRequestHeadSha,
			"pull request head SHA",
		);
		if (sourceCommit !== pullRequestHeadSha) {
			throw new Error(
				`${DIAGNOSTIC_PREFIX} pull request candidate must use the reviewed head SHA`,
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

	return {
		outcome: DEVELOPMENT_PACKAGE_OUTCOMES.INELIGIBLE,
		allowedActions: [],
	};
}

export function assertDevelopmentPackageAction(context, expectedAction) {
	const action = requiredString(expectedAction, "expected action");
	const plan = planDevelopmentPackage(context);
	if (!plan.allowedActions.includes(action)) {
		throw new Error(
			`${DIAGNOSTIC_PREFIX} ${action} is not allowed for outcome ${plan.outcome}`,
		);
	}
	return plan;
}
