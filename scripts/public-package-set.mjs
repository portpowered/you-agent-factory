export const FRONTEND_ONLY_CANDIDATE_SCOPE = "frontend-only";
export const TAGGED_RELEASE_CANDIDATE_SCOPE = "tagged-release";

export const FRONTEND_PUBLIC_PACKAGE_NAMES = Object.freeze([
	"@you-agent-factory/client",
	"@you-agent-factory/factory-replay",
	"@you-agent-factory/factory-emulator",
	"@you-agent-factory/components",
	"@you-agent-factory/factory-visualizers",
]);

export const TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES = Object.freeze([
	"@you-agent-factory/api",
	"@you-agent-factory/packaged-factories",
	...FRONTEND_PUBLIC_PACKAGE_NAMES,
]);

const stableVersionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const publicVersionPattern =
	/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

export function expectedPackageNamesForScope(scope) {
	if (scope === FRONTEND_ONLY_CANDIDATE_SCOPE) {
		return FRONTEND_PUBLIC_PACKAGE_NAMES;
	}
	if (scope === TAGGED_RELEASE_CANDIDATE_SCOPE) {
		return TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES;
	}
	throw new Error(
		`[public-package-set] unknown candidate scope: ${scope ?? "missing"}`,
	);
}

export function assertCandidateSetEvidence(evidence, expectedScope) {
	const expectedNames = expectedPackageNamesForScope(evidence?.scope);
	if (expectedScope !== undefined && evidence.scope !== expectedScope) {
		throw new Error(
			`[public-package-set] expected ${expectedScope} candidate scope, found ${evidence.scope}`,
		);
	}
	const versionPattern =
		evidence.scope === TAGGED_RELEASE_CANDIDATE_SCOPE
			? stableVersionPattern
			: publicVersionPattern;
	if (
		typeof evidence.version !== "string" ||
		!versionPattern.test(evidence.version)
	) {
		throw new Error(
			`[public-package-set] invalid ${evidence.scope} candidate version: ${evidence.version ?? "missing"}`,
		);
	}
	if (!Array.isArray(evidence.packages)) {
		throw new Error("[public-package-set] candidate packages must be an array");
	}
	if (evidence.packages.length !== expectedNames.length) {
		throw new Error(
			`[public-package-set] candidate count mismatch for ${evidence.scope}: expected ${expectedNames.length}, found ${evidence.packages.length}`,
		);
	}
	const names = evidence.packages.map(({ name }) => name);
	if (new Set(names).size !== names.length) {
		throw new Error(
			"[public-package-set] candidate set contains duplicate package names",
		);
	}
	for (const name of expectedNames) {
		const candidate = evidence.packages.find((entry) => entry.name === name);
		if (!candidate || candidate.version !== evidence.version) {
			throw new Error(
				`[public-package-set] missing exact candidate for ${name}@${evidence.version}`,
			);
		}
	}
	return expectedNames;
}
