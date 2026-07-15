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
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { packAndVerify } from "./api-package-pack.mjs";

export const API_PACKAGE_NAME = "@you-agent-factory/api";
export const DEVELOPMENT_DIST_TAG = "dev";
export const SHORT_SHA_LENGTH = 12;

const STABLE_SEMVER_PATTERN =
	/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const SOURCE_COMMIT_PATTERN = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
const RUN_ID_PATTERN = /^[1-9]\d*$/;

function requireString(value, name) {
	if (typeof value !== "string" || value.length === 0) {
		throw new Error(`[api-package-candidate] ${name} is required`);
	}
	return value;
}

export function deriveCandidateVersion({ baseVersion, runId, sourceCommit }) {
	const validatedBaseVersion = requireString(baseVersion, "base version");
	const validatedRunId = requireString(runId, "run ID");
	const validatedSourceCommit = requireString(sourceCommit, "source commit");

	if (!STABLE_SEMVER_PATTERN.test(validatedBaseVersion)) {
		throw new Error(
			"[api-package-candidate] base version must be a stable semantic version",
		);
	}
	if (!RUN_ID_PATTERN.test(validatedRunId)) {
		throw new Error(
			"[api-package-candidate] run ID must be a canonical positive integer",
		);
	}
	if (!SOURCE_COMMIT_PATTERN.test(validatedSourceCommit)) {
		throw new Error(
			"[api-package-candidate] source commit must be a full lowercase Git object ID",
		);
	}

	return `${validatedBaseVersion}-dev.${validatedRunId}.${validatedSourceCommit.slice(0, SHORT_SHA_LENGTH)}`;
}

async function sha256(path) {
	return `sha256:${createHash("sha256").update(await readFile(path)).digest("hex")}`;
}

async function readPackageManifest(packageRoot) {
	const manifestPath = join(packageRoot, "package.json");
	let manifest;
	try {
		manifest = JSON.parse(await readFile(manifestPath, "utf8"));
	} catch (error) {
		throw new Error(
			"[api-package-candidate] package manifest is not readable JSON",
			{ cause: error },
		);
	}
	if (manifest.name !== API_PACKAGE_NAME) {
		throw new Error(
			`[api-package-candidate] package name must be ${API_PACKAGE_NAME}`,
		);
	}
	return { manifest, manifestPath };
}

async function requireEmptyOutputDirectory(outputRoot) {
	await mkdir(outputRoot, { recursive: true });
	if ((await readdir(outputRoot)).length !== 0) {
		throw new Error(
			"[api-package-candidate] output directory must be empty",
		);
	}
}

export async function prepareCandidate({
	packageDirectory,
	outputDirectory,
	runId,
	sourceCommit,
}) {
	const packageRoot = resolve(requireString(packageDirectory, "package directory"));
	const outputRoot = resolve(requireString(outputDirectory, "output directory"));
	const { manifest } = await readPackageManifest(packageRoot);
	const candidateVersion = deriveCandidateVersion({
		baseVersion: manifest.version,
		runId,
		sourceCommit,
	});

	await requireEmptyOutputDirectory(outputRoot);
	const stagingRoot = await mkdtemp(join(tmpdir(), "you-api-candidate-"));
	const stagedPackage = join(stagingRoot, "package");
	try {
		await cp(packageRoot, stagedPackage, { recursive: true });
		const stagedManifestPath = join(stagedPackage, "package.json");
		await writeFile(
			stagedManifestPath,
			`${JSON.stringify({ ...manifest, version: candidateVersion }, null, 2)}\n`,
		);

		const packed = await packAndVerify({
			packageDirectory: stagedPackage,
			packDestination: outputRoot,
		});
		if (
			packed.packageName !== API_PACKAGE_NAME ||
			packed.packageVersion !== candidateVersion
		) {
			throw new Error(
				"[api-package-candidate] packed identity does not match the candidate",
			);
		}

		const evidence = {
			packageName: packed.packageName,
			candidateVersion,
			sourceCommit,
			contractDigest: await sha256(
				join(stagedPackage, "generated", "manifest.json"),
			),
			artifactDigest: await sha256(packed.tarballPath),
			inventory: packed.files,
			distTag: DEVELOPMENT_DIST_TAG,
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
