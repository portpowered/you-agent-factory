import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";
import { timePackagePhase } from "./package-phase-timing.mjs";
import {
	packCandidate,
	preparePublicPackageCandidates,
} from "../ui/scripts/public-package-publish.mjs";
import { packAndVerify as packApi } from "./api-package-pack.mjs";
import { packAndVerify as packFactories } from "./packaged-factories-package-pack.mjs";
import {
	normalizeStagedMtimes,
	prepareReleaseCandidate,
} from "./package-release-candidate.mjs";
import {
	assertCandidateSetEvidence,
	TAGGED_RELEASE_CANDIDATE_SCOPE,
} from "./public-package-set.mjs";

export { TAGGED_RELEASE_CANDIDATE_SCOPE };

// Keep the reviewed export/inventory checks while packing the staged artifact with Bun.
async function bunPack(packageDirectory, packDestination) {
	const manifestPath = join(packageDirectory, "package.json");
	const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
	if (manifest.name === "@you-agent-factory/packaged-factories") {
		// npm implicitly includes nested README files; Bun requires explicit inclusion.
		await writeFile(
			manifestPath,
			`${JSON.stringify({ ...manifest, files: [...new Set([...manifest.files, "generated/README.md"])] }, null, 2)}\n`,
		);
		await normalizeStagedMtimes(packageDirectory);
	}
	return (
		await packCandidate({
			stagedDirectory: packageDirectory,
			outputDirectory: packDestination,
		})
	).stdout;
}

export function prepareDataPackageCandidate(input) {
	const pack =
		input.packageName === "@you-agent-factory/api" ? packApi : packFactories;
	return prepareReleaseCandidate({
		...input,
		pack: (options) => pack({ ...options, npmPack: bunPack }),
	});
}

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

export async function prepareTaggedReleaseCandidate(
	{
		outputDirectory,
		runId,
		sourceCommit,
		version,
		apiPackageDirectory = "packages/api",
		packagedFactoriesPackageDirectory = "packages/packaged-factories",
	},
	dependencies = {},
) {
	const root = resolve(outputDirectory);
	await requireEmptyOutputDirectory(root);
	const apiOutput = join(root, "api");
	const packagedFactoriesOutput = join(root, "packaged-factories");
	const frontendOutput = join(root, "frontend");
	const [api, packagedFactories, frontend] = await Promise.all([
		timePackagePhase("tagged api stage/pack", () =>
			(dependencies.prepareData ?? prepareDataPackageCandidate)({
				packageName: "@you-agent-factory/api",
				packageDirectory: apiPackageDirectory,
				outputDirectory: apiOutput,
				runId,
				sourceCommit,
				version,
				distTag: "latest",
			}),
		),
		timePackagePhase("tagged factories stage/pack", () =>
			(dependencies.prepareData ?? prepareDataPackageCandidate)({
				packageName: "@you-agent-factory/packaged-factories",
				packageDirectory: packagedFactoriesPackageDirectory,
				outputDirectory: packagedFactoriesOutput,
				runId,
				sourceCommit,
				version,
				distTag: "latest",
			}),
		),
		timePackagePhase("tagged frontend build/stage/pack", () =>
			(dependencies.prepareFrontend ?? preparePublicPackageCandidates)({
				version,
				outputDirectory: frontendOutput,
			}),
		),
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
