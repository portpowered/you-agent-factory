export const BACKEND_LINT_BASELINE_SOURCE =
	"Hosted Backend Lint run 31870641599 on 2026-08-15 (rebased current-main baseline head 925e9d74a7702bd6b562812fb8db4ac47348bb4f)";

// These are measured current-main failures, not extra capacity. A failing
// checker is allowed only while its observed count remains at or below the
// recorded count; a clean checker and any unlisted failure remain gated.
export const BACKEND_LINT_ALLOWANCES = Object.freeze({
	"ui-deadcode": {
		baselineViolationCount: 12,
		reason: "The hosted baseline found pre-existing unused frontend test-support fixtures.",
		ownerOrLane: "Frontend dead-code cleanup lane",
		deadline: "2026-09-30",
		removalCondition: "Remove the unused fixtures, then delete this allowance when hosted ui-deadcode reports zero violations.",
	},
	"backend-size": {
		baselineViolationCount: 2,
		reason: "Two pre-existing tests exceed the unchanged 100-line function limit.",
		ownerOrLane: "Backend-size remediation lane; preserve cmd/gocoveragecheck ownership from PR #1980",
		deadline: "2026-09-30",
		removalCondition: "Split the two oversized tests without changing the checker limit, then delete this allowance when hosted backend-size passes.",
	},
	"package-target-manifest-check": {
		baselineViolationCount: 3,
		reason: "The current package inventory omits the factory_definitions runtime_snapshot package entries.",
		ownerOrLane: "Factory Definitions package inventory remediation lane",
		deadline: "2026-10-15",
		removalCondition: "Reconcile the package inventory with the production tree, then delete this allowance when hosted package-target-manifest-check passes.",
	},
	"packaged-factory-consumption-check": {
		baselineViolationCount: 4,
		reason: "The migration-ledger matrix directly imports packaged-factories instead of using the catalog boundary.",
		ownerOrLane: "Packaged-factory consumption-boundary remediation lane",
		deadline: "2026-10-15",
		removalCondition: "Route the matrix through the Factory Definitions catalog boundary, then delete this allowance when hosted packaged-factory-consumption-check passes.",
	},
	"ownership-inventory-check": {
		baselineViolationCount: 5,
		reason: "The 2026-08-08 packaged-service-structure migration debt leaves the ownership inventory and cross-service edges incomplete.",
		ownerOrLane: "Packaged-service structure migration / ownership inventory lane",
		deadline: "2026-10-31",
		removalCondition: "Reconcile the migrated package and edge inventory, then delete this allowance when hosted ownership-inventory-check passes.",
	},
	deadcode: {
		baselineViolationCount: 580,
		reason: "The hosted baseline reports 580 existing dead-code findings against the repository baseline.",
		ownerOrLane: "Repository dead-code cleanup lane",
		deadline: "2026-12-31",
		removalCondition: "Remove or review the existing findings and update the source baseline intentionally, then delete this allowance when hosted deadcode reports zero drift.",
	},
});

function allowanceStatus(target, allowance) {
	if (target.status === "pass") {
		return "clean";
	}
	if (!allowance) {
		return "new failure";
	}
	return target.violationCount <= allowance.baselineViolationCount
		? "allowed"
		: "exceeded";
}

export function evaluateBackendLintPolicy(targets) {
	const failures = [];
	const evaluatedTargets = targets.map((target) => {
		const allowance = BACKEND_LINT_ALLOWANCES[target.name];
		const status = allowanceStatus(target, allowance);
		if (status === "new failure") {
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
	};
}
