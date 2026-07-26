import { mkdir, readdir, writeFile } from "node:fs/promises";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";
import { preparePublicPackageCandidates } from "../ui/scripts/public-package-publish.mjs";
import { prepareCandidate as prepareApiCandidate } from "./api-package-candidate.mjs";
import { prepareCandidate as preparePackagedFactoriesCandidate } from "./packaged-factories-package-candidate.mjs";
import {
	assertCandidateSetEvidence,
	TAGGED_RELEASE_CANDIDATE_SCOPE,
} from "./public-package-set.mjs";

export { TAGGED_RELEASE_CANDIDATE_SCOPE };

async function requireEmptyOutputDirectory(outputDirectory) {
	await mkdir(outputDirectory, { recursive: true });
	if ((await readdir(outputDirectory)).length !== 0) {
		throw new Error(
			"[public-release-package-candidate] output directory must be empty",
		);
	}
}

function candidateRecord({ name, version, tarballPath, outputDirectory }) {
	const filename = basename(tarballPath);
	return {
		name,
		version,
		tarball: join(outputDirectory, filename).replaceAll("\\", "/"),
	};
}

export async function prepareTaggedReleaseCandidate({
	outputDirectory,
	runId,
	sourceCommit,
	version,
	apiPackageDirectory = "packages/api",
	packagedFactoriesPackageDirectory = "packages/packaged-factories",
}) {
	const root = resolve(outputDirectory);
	await requireEmptyOutputDirectory(root);
	const apiOutput = join(root, "api");
	const packagedFactoriesOutput = join(root, "packaged-factories");
	const frontendOutput = join(root, "frontend");
	const [api, packagedFactories, frontend] = await Promise.all([
		prepareApiCandidate({
			packageDirectory: apiPackageDirectory,
			outputDirectory: apiOutput,
			runId,
			sourceCommit,
			version,
			distTag: "latest",
		}),
		preparePackagedFactoriesCandidate({
			packageDirectory: packagedFactoriesPackageDirectory,
			outputDirectory: packagedFactoriesOutput,
			runId,
			sourceCommit,
			version,
			distTag: "latest",
		}),
		preparePublicPackageCandidates({
			version,
			outputDirectory: frontendOutput,
		}),
	]);
	const packages = [
		candidateRecord({
			name: api.evidence.packageName,
			version: api.evidence.candidateVersion,
			tarballPath: api.tarballPath,
			outputDirectory: "api",
		}),
		candidateRecord({
			name: packagedFactories.evidence.packageName,
			version: packagedFactories.evidence.candidateVersion,
			tarballPath: packagedFactories.tarballPath,
			outputDirectory: "packaged-factories",
		}),
		...frontend.packages.map(({ name, version: candidateVersion, filename }) =>
			candidateRecord({
				name,
				version: candidateVersion,
				tarballPath: filename,
				outputDirectory: "frontend",
			}),
		),
	];
	const evidence = {
		scope: TAGGED_RELEASE_CANDIDATE_SCOPE,
		version,
		sourceCommit,
		packages,
	};
	assertCandidateSetEvidence(evidence, TAGGED_RELEASE_CANDIDATE_SCOPE);
	const evidencePath = join(root, "release-candidate-evidence.json");
	await writeFile(evidencePath, `${JSON.stringify(evidence, null, 2)}\n`);
	return { evidence, evidencePath };
}

async function main() {
	const { values } = parseArgs({
		options: {
			"output-directory": { type: "string" },
			"run-id": { type: "string" },
			"source-commit": { type: "string" },
			version: { type: "string" },
		},
		strict: true,
	});
	const result = await prepareTaggedReleaseCandidate({
		outputDirectory: values["output-directory"],
		runId: values["run-id"],
		sourceCommit: values["source-commit"],
		version: values.version,
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
