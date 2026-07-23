import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { installAndVerifyRegistryPackage } from "./api-package-consumer.mjs";
import {
	createNpmRegistryClient,
	reconcileCandidate,
} from "./api-package-registry.mjs";
import {
	PackagePublicationError,
	PUBLICATION_FAILURES,
	PUBLICATION_OUTCOMES,
	publishAndVerifyCandidate as publishAndVerifyPackageCandidate,
	publishCandidateTarball,
	publishCandidateDirectory as publishPackageCandidateDirectory,
} from "./package-publication.mjs";

export {
	PackagePublicationError,
	PUBLICATION_FAILURES,
	PUBLICATION_OUTCOMES,
	publishCandidateTarball,
};

function productionDependencies(overrides = {}) {
	return {
		consumerDirectoryPrefix: "you-api-registry-consumer-",
		createNpmRegistryClient,
		installAndVerifyRegistryPackage,
		reconcileCandidate,
		...overrides,
	};
}

export function publishAndVerifyCandidate(input, dependencies = {}) {
	return publishAndVerifyPackageCandidate(
		input,
		productionDependencies(dependencies),
	);
}

export function publishCandidateDirectory(input, dependencies = {}) {
	return publishPackageCandidateDirectory(
		input,
		productionDependencies(dependencies),
	);
}

async function main() {
	const { values } = parseArgs({
		options: {
			"candidate-directory": { type: "string" },
			"expected-source-commit": { type: "string" },
			"workspace-directory": { type: "string" },
		},
		strict: true,
	});
	const result = await publishCandidateDirectory({
		candidateDirectory: values["candidate-directory"],
		expectedSourceCommit: values["expected-source-commit"],
		workspaceDirectory: values["workspace-directory"],
	});
	process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (
	process.argv[1] &&
	import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
	main().catch((error) => {
		const code = error?.code ?? PUBLICATION_FAILURES.PUBLISH_FAILED;
		process.stderr.write(`${code}: ${error.message}\n`);
		process.exitCode = 1;
	});
}
