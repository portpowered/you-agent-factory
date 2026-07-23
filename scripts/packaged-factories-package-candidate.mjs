import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { packPackage } from "./api-package-pack.mjs";
import {
	DEVELOPMENT_DIST_TAG,
	deriveCandidateVersion,
	prepareReleaseCandidate,
} from "./package-release-candidate.mjs";

export const PACKAGED_FACTORIES_PACKAGE_NAME =
	"@you-agent-factory/packaged-factories";

export { DEVELOPMENT_DIST_TAG, deriveCandidateVersion };

export async function prepareCandidate({
	packageDirectory,
	outputDirectory,
	runId,
	sourceCommit,
}) {
	return prepareReleaseCandidate({
		packageDirectory,
		outputDirectory,
		runId,
		sourceCommit,
		packageName: PACKAGED_FACTORIES_PACKAGE_NAME,
		pack: packPackage,
	});
}

async function main() {
	const { values } = parseArgs({
		options: {
			"output-directory": { type: "string" },
			"package-directory": { type: "string" },
			"run-id": { type: "string" },
			"source-commit": { type: "string" },
		},
		strict: true,
	});
	const result = await prepareCandidate({
		packageDirectory: values["package-directory"],
		outputDirectory: values["output-directory"],
		runId: values["run-id"],
		sourceCommit: values["source-commit"],
	});
	process.stdout.write(`${JSON.stringify(result.evidence)}\n`);
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
