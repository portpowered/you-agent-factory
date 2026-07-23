import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import {
	API_PACKAGE_NAME,
	DEVELOPMENT_DIST_TAG,
} from "./api-package-candidate.mjs";
import {
	createNpmRegistryClient,
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	RegistryReconciliationError,
	reconcileCandidate as reconcilePackageCandidate,
} from "./package-registry.mjs";

export {
	createNpmRegistryClient,
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	RegistryReconciliationError,
};

export function reconcileCandidate(input) {
	return reconcilePackageCandidate({
		...input,
		expectedPackageName: API_PACKAGE_NAME,
		expectedDistTag: DEVELOPMENT_DIST_TAG,
	});
}

async function main() {
	const { values } = parseArgs({
		options: {
			"evidence-file": { type: "string" },
			"tarball-file": { type: "string" },
		},
		strict: true,
	});
	const evidence = JSON.parse(await readFile(values["evidence-file"], "utf8"));
	const result = await reconcileCandidate({
		evidence,
		tarballPath: values["tarball-file"],
		registryClient: createNpmRegistryClient(),
	});
	process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (
	process.argv[1] &&
	import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
	main().catch((error) => {
		const code = error?.code ?? RECONCILIATION_FAILURES.CANDIDATE_INVALID;
		process.stderr.write(`${code}: ${error.message}\n`);
		process.exitCode = 1;
	});
}
