export const BACKEND_LINT_BASELINE_SOURCE =
	"Hosted Backend Lint run 31974329925 on 2026-08-16 (semantic baseline observed on rebased head a4b25a57198454083670ae632d0111502e073244)";

// These are measured current-main failures, not extra capacity. A failing
// checker is allowed only while its observed count remains at or below the
// recorded count; a clean checker and any unlisted failure remain gated.
export const BACKEND_LINT_ALLOWANCES = Object.freeze({});

// Targets recorded here are gated with no allowance at all: any failure is a
// policy failure on its first run, and the target must appear in every Backend
// Lint report. Requiring observation matters for ratchet checkers, because
// dropping a target from LINT_TARGETS would otherwise be a silent way to turn
// a red ratchet green without doing the decoupling work it is measuring.
export const BACKEND_LINT_REQUIRED_TARGETS = Object.freeze({
	golangci: {
		reason: "Built-in and shared compiler diagnostics, including live CLI manifest authority, must run without any report allowance.",
		ownerOrLane: "Shared repository lint",
	},
	deadcode: {
		reason: "Exact normalized backend dead-code finding set: the committed baseline is the sole durable tolerance source; selected inputs and legacy reports require the analyzer.",
		ownerOrLane: "Backend dead-code gate",
	},
});

function allowanceStatus(target, allowance) {
	if (target.status === "pass") {
		return "clean";
	}
	if (!Number.isSafeInteger(target.violationCount) || target.violationCount < 0) {
		return "unmeasured";
	}
	if (!allowance) {
		return "new failure";
	}
	return target.violationCount <= allowance.baselineViolationCount
		? "allowed"
		: "exceeded";
}

export function evaluateBackendLintPolicy(targets, selection = null) {
	const failures = [];
	const evaluatedTargets = targets.map((target) => {
		const allowance = BACKEND_LINT_ALLOWANCES[target.name];
		const status = allowanceStatus(target, allowance);
		if (status === "unmeasured") {
			failures.push(
				`${target.name} failed without a reliable machine-readable violation count; its baseline allowance cannot be applied.`,
			);
		} else if (status === "new failure") {
			failures.push(
				`${target.name} failed with ${target.violationCount} reported violation(s); no baseline allowance exists.`,
			);
		} else if (status === "exceeded") {
			failures.push(
				`${target.name} reported ${target.violationCount} violation(s), exceeding its baseline allowance of ${allowance.baselineViolationCount}.`,
			);
		}

		return {
			...target,
			policyStatus: status,
			baselineViolationCount: allowance?.baselineViolationCount ?? null,
			allowance,
		};
	});

	for (const name of Object.keys(BACKEND_LINT_ALLOWANCES)) {
		if (!evaluatedTargets.some((target) => target.name === name)) {
			failures.push(`${name} has a baseline allowance but was not observed in the lint report.`);
		}
	}

	const requirements = { ...BACKEND_LINT_REQUIRED_TARGETS };
	if (selection) {
		if (selection.deadcode === 0) delete requirements.deadcode;
		if (selection.docs === 1) requirements["docs-reference-check"] = { reason: "Selected documentation inputs must be checked without allowance.", ownerOrLane: "Documentation lint" };
		for (const [name, key] of [["deadcode", "deadcode"], ["docs-reference-check", "docs"]]) {
			if (selection[key] === 0 && targets.some((target) => target.name === name)) failures.push(`${name} was declared skipped but was observed.`);
		}
	}
	const requiredTargets = Object.entries(requirements).map(([name, requirement]) => {
		const target = evaluatedTargets.find((item) => item.name === name);
		if (BACKEND_LINT_ALLOWANCES[name]) {
			failures.push(`${name} is gated with no allowance but a baseline allowance was recorded for it; remove one of the two entries.`);
		}
		if (!target) {
			failures.push(`${name} is gated with no allowance and must run in every lint report, but it was not observed.`);
		}
		return {
			name,
			...requirement,
			observedViolationCount: target?.violationCount ?? null,
			status: target ? target.policyStatus : "not observed",
		};
	});

	const allowances = Object.entries(BACKEND_LINT_ALLOWANCES).map(([name, allowance]) => {
		const target = evaluatedTargets.find((item) => item.name === name);
		return {
			name,
			...allowance,
			observedViolationCount: target?.violationCount ?? null,
			status: target ? allowanceStatus(target, allowance) : "not observed",
		};
	});

	return {
		ok: failures.length === 0,
		failures,
		targets: evaluatedTargets,
		allowances,
		requiredTargets,
	};
}
