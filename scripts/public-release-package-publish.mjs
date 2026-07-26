import { createHash } from "node:crypto";
import { readdir, readFile } from "node:fs/promises";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";
import { publishPublicPackageCandidates } from "../ui/scripts/public-package-publish.mjs";
import { API_PACKAGE_NAME } from "./api-package-candidate.mjs";
import { publishCandidateDirectory as publishApiCandidate } from "./api-package-publish.mjs";
import { PACKAGED_FACTORIES_PACKAGE_NAME } from "./packaged-factories-package-candidate.mjs";
import { publishCandidateDirectory as publishPackagedFactoriesCandidate } from "./packaged-factories-package-publish.mjs";
import {
	assertCandidateSetEvidence,
	FRONTEND_ONLY_CANDIDATE_SCOPE,
	TAGGED_RELEASE_CANDIDATE_SCOPE,
} from "./public-package-set.mjs";

async function readJson(path) {
	return JSON.parse(await readFile(path, "utf8"));
}

async function singleTarballName(directory) {
	const tarballs = (await readdir(directory)).filter((name) =>
		name.endsWith(".tgz"),
	);
	if (tarballs.length !== 1) {
		throw new Error(
			`[public-release-package-publish] ${directory} must contain exactly one tarball`,
		);
	}
	return tarballs[0];
}

function digest(contents, algorithm, encoding = "hex") {
	return createHash(algorithm).update(contents).digest(encoding);
}

async function assertTarballDigests({
	name,
	tarballPath,
	artifactDigest,
	integrity,
	shasum,
}) {
	let contents;
	try {
		contents = await readFile(tarballPath);
	} catch (error) {
		throw new Error(
			`[public-release-package-publish] represented tarball is not readable for ${name}`,
			{ cause: error },
		);
	}
	if (
		artifactDigest !== undefined &&
		artifactDigest !== `sha256:${digest(contents, "sha256")}`
	) {
		throw new Error(
			`[public-release-package-publish] candidate digest mismatch for ${name}`,
		);
	}
	if (shasum !== undefined && shasum !== digest(contents, "sha1")) {
		throw new Error(
			`[public-release-package-publish] candidate shasum mismatch for ${name}`,
		);
	}
	if (
		integrity !== undefined &&
		integrity !== `sha512-${digest(contents, "sha512", "base64")}`
	) {
		throw new Error(
			`[public-release-package-publish] candidate integrity mismatch for ${name}`,
		);
	}
}

function assertTopLevelRecord(evidence, name, version, tarball) {
	const record = evidence.packages.find((candidate) => candidate.name === name);
	if (record?.version !== version || record.tarball !== tarball) {
		throw new Error(
			`[public-release-package-publish] top-level evidence does not represent ${name}@${version} at ${tarball}`,
		);
	}
}

async function assertSinglePackageEvidence({
	root,
	directory,
	expectedName,
	evidence,
}) {
	const candidateDirectory = join(root, directory);
	const child = await readJson(
		join(candidateDirectory, "candidate-evidence.json"),
	);
	const tarballName = await singleTarballName(candidateDirectory);
	if (
		child.packageName !== expectedName ||
		child.candidateVersion !== evidence.version ||
		child.sourceCommit !== evidence.sourceCommit ||
		child.distTag !== "latest" ||
		!/^sha256:[0-9a-f]{64}$/.test(child.contractDigest ?? "") ||
		!/^sha256:[0-9a-f]{64}$/.test(child.artifactDigest ?? "") ||
		!Array.isArray(child.inventory) ||
		child.inventory.some((path) => typeof path !== "string")
	) {
		throw new Error(
			`[public-release-package-publish] child evidence does not match ${expectedName}`,
		);
	}
	assertTopLevelRecord(
		evidence,
		expectedName,
		child.candidateVersion,
		`${directory}/${tarballName}`,
	);
	await assertTarballDigests({
		name: expectedName,
		tarballPath: join(candidateDirectory, tarballName),
		artifactDigest: child.artifactDigest,
	});
}

async function assertFrontendEvidence(root, evidence) {
	const child = await readJson(
		join(root, "frontend", "public-package-candidates.json"),
	);
	assertCandidateSetEvidence(child, FRONTEND_ONLY_CANDIDATE_SCOPE);
	if (child.version !== evidence.version) {
		throw new Error(
			"[public-release-package-publish] frontend evidence version does not match tagged release",
		);
	}
	for (const candidate of child.packages) {
		if (
			typeof candidate.filename !== "string" ||
			basename(candidate.filename) !== candidate.filename ||
			!/^[0-9a-f]{40}$/.test(candidate.shasum ?? "") ||
			!/^sha512-[A-Za-z0-9+/]+={0,2}$/.test(candidate.integrity ?? "")
		) {
			throw new Error(
				`[public-release-package-publish] invalid frontend tarball evidence for ${candidate.name}`,
			);
		}
		assertTopLevelRecord(
			evidence,
			candidate.name,
			candidate.version,
			`frontend/${candidate.filename}`,
		);
		await assertTarballDigests({
			name: candidate.name,
			tarballPath: join(root, "frontend", candidate.filename),
			integrity: candidate.integrity,
			shasum: candidate.shasum,
		});
	}
}

export async function validateTaggedReleaseCandidate({
	candidateDirectory,
	expectedSourceCommit,
}) {
	const root = resolve(candidateDirectory);
	const evidence = await readJson(
		join(root, "release-candidate-evidence.json"),
	);
	assertCandidateSetEvidence(evidence, TAGGED_RELEASE_CANDIDATE_SCOPE);
	if (evidence.sourceCommit !== expectedSourceCommit) {
		throw new Error(
			"[public-release-package-publish] preserved candidate source commit does not match the protected workflow commit",
		);
	}
	await Promise.all([
		assertSinglePackageEvidence({
			root,
			directory: "api",
			expectedName: API_PACKAGE_NAME,
			evidence,
		}),
		assertSinglePackageEvidence({
			root,
			directory: "packaged-factories",
			expectedName: PACKAGED_FACTORIES_PACKAGE_NAME,
			evidence,
		}),
		assertFrontendEvidence(root, evidence),
	]);
	return { evidence, root };
}

export async function publishTaggedReleaseCandidate(
	{ candidateDirectory, expectedSourceCommit, workspaceDirectory },
	dependencies = {},
) {
	const publishApi = dependencies.publishApiCandidate ?? publishApiCandidate;
	const publishPackagedFactories =
		dependencies.publishPackagedFactoriesCandidate ??
		publishPackagedFactoriesCandidate;
	const publishFrontend =
		dependencies.publishFrontendCandidates ?? publishPublicPackageCandidates;
	const { evidence, root } = await validateTaggedReleaseCandidate({
		candidateDirectory,
		expectedSourceCommit,
	});
	const api = await publishApi({
		candidateDirectory: join(root, "api"),
		expectedDistTag: "latest",
		expectedSourceCommit,
		workspaceDirectory,
	});
	const packagedFactories = await publishPackagedFactories({
		candidateDirectory: join(root, "packaged-factories"),
		expectedDistTag: "latest",
		expectedSourceCommit,
		workspaceDirectory,
	});
	const frontend = await publishFrontend({
		candidateDirectory: join(root, "frontend"),
		tag: "latest",
		provenance: true,
	});
	return { evidence, publications: { api, packagedFactories, frontend } };
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
	const result = await publishTaggedReleaseCandidate({
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
		process.stderr.write(`${error.message}\n`);
		process.exitCode = 1;
	});
}
