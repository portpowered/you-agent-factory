import {
	createNpmRegistryClient,
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	RegistryReconciliationError,
	reconcileCandidate as reconcilePackageCandidate,
} from "./package-registry.mjs";
import {
	DEVELOPMENT_DIST_TAG,
	PACKAGED_FACTORIES_PACKAGE_NAME,
} from "./packaged-factories-package-candidate.mjs";

export {
	createNpmRegistryClient,
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	RegistryReconciliationError,
};

export function reconcileCandidate(input) {
	return reconcilePackageCandidate({
		...input,
		expectedPackageName: PACKAGED_FACTORIES_PACKAGE_NAME,
		expectedDistTag: input.expectedDistTag ?? DEVELOPMENT_DIST_TAG,
	});
}
