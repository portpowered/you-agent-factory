import { createHash } from "node:crypto";
import {
	cp,
	mkdir,
	mkdtemp,
	readFile,
	readdir,
	rm,
	writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

export const DEVELOPMENT_DIST_TAG = "dev";
export const SHORT_SHA_LENGTH = 12;

const STABLE_SEMVER_PATTERN =
	/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const SOURCE_COMMIT_PATTERN = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
const RUN_ID_PATTERN = /^[1-9]\d*$/;

function requireString(value, name) {
	if (typeof value !== "string" || value.length === 0) {
		throw new Error(`[package-release-candidate] ${name} is required`);
	}
	return value;
}

export function deriveCandidateVersion({ baseVersion, runId, sourceCommit }) {
	const validatedBaseVersion = requireString(baseVersion, "base version");
	const validatedRunId = requireString(runId, "run ID");
	const validatedSourceCommit = requireString(sourceCommit, "source commit");

	if (!STABLE_SEMVER_PATTERN.test(validatedBaseVersion)) {
		throw new Error(
			"[package-release-candidate] base version must be a stable semantic version",
		);
	}
	if (!RUN_ID_PATTERN.test(validatedRunId)) {
		throw new Error(
			"[package-release-candidate] run ID must be a canonical positive integer",
		);
	}
	if (!SOURCE_COMMIT_PATTERN.test(validatedSourceCommit)) {
		throw new Error(
			"[package-release-candidate] source commit must be a full lowercase Git object ID",
		);
	}

	return `${validatedBaseVersion}-dev.${validatedRunId}.${validatedSourceCommit.slice(0, SHORT_SHA_LENGTH)}`;
}

async function sha256(path) {
	return `sha256:${createHash("sha256").update(await readFile(path)).digest("hex")}`;
}

async function readJSON(path, description) {
	try {
		return JSON.parse(await readFile(path, "utf8"));
	} catch (error) {
		throw new Error(
			`[package-release-candidate] ${description} is not readable JSON`,
			{ cause: error },
		);
	}
}

async function requireEmptyOutputDirectory(outputRoot) {
	await mkdir(outputRoot, { recursive: true });
	if ((await readdir(outputRoot)).length !== 0) {
		throw new Error(
			"[package-release-candidate] output directory must be empty",
		);
	}
}

async function stageProvenance(stagedPackage, contractManifestPath, sourceCommit) {
	const manifestPath = join(stagedPackage, ...contractManifestPath.split("/"));
	const manifest = await readJSON(manifestPath, "contract manifest");
	await writeFile(
		manifestPath,
		`${JSON.stringify({ ...manifest, sourceCommit }, null, 2)}\n`,
	);
	const stagedManifest = await readJSON(manifestPath, "staged contract manifest");
	if (stagedManifest.sourceCommit !== sourceCommit) {
		throw new Error(
			"[package-release-candidate] staged provenance does not match source commit",
		);
	}
	return manifestPath;
}

export async function prepareReleaseCandidate({
	packageDirectory,
	outputDirectory,
	runId,
	sourceCommit,
	packageName,
	contractManifestPath = "generated/manifest.json",
	distTag = DEVELOPMENT_DIST_TAG,
	pack,
}) {
	const expectedPackageName = requireString(packageName, "package name");
	const packageRoot = resolve(requireString(packageDirectory, "package directory"));
	const outputRoot = resolve(requireString(outputDirectory, "output directory"));
	const validatedDistTag = requireString(distTag, "distribution tag");
	if (typeof pack !== "function") {
		throw new Error("[package-release-candidate] pack function is required");
	}

	const manifest = await readJSON(
		join(packageRoot, "package.json"),
		"package manifest",
	);
	if (manifest.name !== expectedPackageName) {
		throw new Error(
			`[package-release-candidate] package name must be ${expectedPackageName}`,
		);
	}
	const candidateVersion = deriveCandidateVersion({
		baseVersion: manifest.version,
		runId,
		sourceCommit,
	});

	await requireEmptyOutputDirectory(outputRoot);
	const stagingRoot = await mkdtemp(
		join(tmpdir(), "you-package-release-candidate-"),
	);
	const stagedPackage = join(stagingRoot, "package");
	try {
		await cp(packageRoot, stagedPackage, { recursive: true });
		await writeFile(
			join(stagedPackage, "package.json"),
			`${JSON.stringify({ ...manifest, version: candidateVersion }, null, 2)}\n`,
		);
		const stagedContractManifest = await stageProvenance(
			stagedPackage,
			contractManifestPath,
			sourceCommit,
		);

		const packed = await pack({
			packageDirectory: stagedPackage,
			packDestination: outputRoot,
		});
		if (
			packed.packageName !== expectedPackageName ||
			packed.packageVersion !== candidateVersion
		) {
			throw new Error(
				"[package-release-candidate] packed identity does not match the candidate",
			);
		}

		const evidence = {
			packageName: packed.packageName,
			candidateVersion,
			sourceCommit,
			contractDigest: await sha256(stagedContractManifest),
			artifactDigest: await sha256(packed.tarballPath),
			inventory: [...packed.files].sort((left, right) =>
				left.localeCompare(right),
			),
			distTag: validatedDistTag,
		};
		const evidencePath = join(outputRoot, "candidate-evidence.json");
		await writeFile(evidencePath, `${JSON.stringify(evidence, null, 2)}\n`);

		return {
			evidence,
			evidencePath,
			tarballPath: packed.tarballPath,
		};
	} finally {
		await rm(stagingRoot, { recursive: true, force: true });
	}
}
