import { spawn } from "node:child_process";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { installAndVerifyRegistryPackage } from "./api-package-consumer.mjs";
import {
	RECONCILIATION_OUTCOMES,
	createNpmRegistryClient,
	reconcileCandidate,
} from "./api-package-registry.mjs";

export const PUBLICATION_OUTCOMES = Object.freeze({
	PUBLISHED_AND_VERIFIED: "PUBLISHED_AND_VERIFIED",
	VERIFIED_EXISTING: "VERIFIED_EXISTING",
});

export const PUBLICATION_FAILURES = Object.freeze({
	AUTHENTICATION_FAILED: "PUBLICATION_AUTHENTICATION_FAILED",
	PERMISSION_FAILED: "PUBLICATION_PERMISSION_FAILED",
	PUBLISH_FAILED: "PUBLICATION_FAILED",
	REGISTRY_VERIFICATION_FAILED: "REGISTRY_VERIFICATION_FAILED",
});

const DEFAULT_VERIFICATION_ATTEMPTS = 6;
const DEFAULT_VERIFICATION_DELAY_MS = 5_000;
const DEFAULT_PUBLISH_TIMEOUT_MS = 120_000;

export class PackagePublicationError extends Error {
	constructor(code, message, options = {}) {
		super(`[api-package-publish] ${message}`, options);
		this.name = "PackagePublicationError";
		this.code = code;
	}
}

function publicationFailure(code, message, cause) {
	return new PackagePublicationError(code, message, { cause });
}

function approvedDiagnostic(evidence, outcome) {
	return {
		packageName: evidence.packageName,
		candidateVersion: evidence.candidateVersion,
		sourceCommit: evidence.sourceCommit,
		contractDigest: evidence.contractDigest,
		artifactDigest: evidence.artifactDigest,
		inventory: evidence.inventory,
		distTag: evidence.distTag,
		outcome,
	};
}

function classifyPublishFailure(output) {
	if (/\b(?:E401|ENEEDAUTH)\b|\b401\b/.test(output)) {
		return publicationFailure(
			PUBLICATION_FAILURES.AUTHENTICATION_FAILED,
			"npm trusted-publishing authentication failed",
		);
	}
	if (/\bE403\b|\b403\b/.test(output)) {
		return publicationFailure(
			PUBLICATION_FAILURES.PERMISSION_FAILED,
			"npm trusted-publishing permission denied",
		);
	}
	return publicationFailure(
		PUBLICATION_FAILURES.PUBLISH_FAILED,
		"npm publish failed",
	);
}

export function publishCandidateTarball({
	tarballPath,
	distTag,
	timeoutMs = DEFAULT_PUBLISH_TIMEOUT_MS,
}) {
	return new Promise((resolvePromise, rejectPromise) => {
		const child = spawn(
			"npm",
			[
				"publish",
				resolve(tarballPath),
				"--access",
				"public",
				"--provenance",
				"--tag",
				distTag,
			],
			{ shell: process.platform === "win32", stdio: ["ignore", "pipe", "pipe"] },
		);
		let output = "";
		const timeout = setTimeout(() => child.kill(), timeoutMs);
		child.stdout.setEncoding("utf8");
		child.stderr.setEncoding("utf8");
		child.stdout.on("data", (chunk) => {
			output += chunk;
		});
		child.stderr.on("data", (chunk) => {
			output += chunk;
		});
		child.on("error", () => {
			clearTimeout(timeout);
			rejectPromise(
				publicationFailure(
					PUBLICATION_FAILURES.PUBLISH_FAILED,
					"npm publish could not start",
				),
			);
		});
		child.on("close", (status) => {
			clearTimeout(timeout);
			if (status === 0) {
				resolvePromise();
				return;
			}
			rejectPromise(classifyPublishFailure(output));
		});
	});
}

function wait(delayMs) {
	return new Promise((resolvePromise) => setTimeout(resolvePromise, delayMs));
}

async function verifyPublishedRegistryState({
	evidence,
	tarballPath,
	registryClient,
	reconcile,
	verificationAttempts,
	verificationDelayMs,
	sleep,
}) {
	for (let attempt = 1; attempt <= verificationAttempts; attempt += 1) {
		const result = await reconcile({ evidence, tarballPath, registryClient });
		if (result.outcome === RECONCILIATION_OUTCOMES.VERIFIED_EXISTING) {
			return;
		}
		if (attempt < verificationAttempts) {
			await sleep(verificationDelayMs);
		}
	}
	throw publicationFailure(
		PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED,
		"published version did not become digest-verifiable within the bounded verification window",
	);
}

export async function publishAndVerifyCandidate(
	{
		consumerDirectory,
		evidence,
		registryClient,
		tarballPath,
		workspaceDirectory,
		verificationAttempts = DEFAULT_VERIFICATION_ATTEMPTS,
		verificationDelayMs = DEFAULT_VERIFICATION_DELAY_MS,
	},
	dependencies = {},
) {
	const reconcile = dependencies.reconcileCandidate ?? reconcileCandidate;
	const publish = dependencies.publishCandidateTarball ?? publishCandidateTarball;
	const install =
		dependencies.installAndVerifyRegistryPackage ??
		installAndVerifyRegistryPackage;
	const sleep = dependencies.sleep ?? wait;
	const initial = await reconcile({ evidence, tarballPath, registryClient });
	let outcome = PUBLICATION_OUTCOMES.VERIFIED_EXISTING;

	if (initial.outcome === RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED) {
		let publishError;
		try {
			await publish({ tarballPath, distTag: evidence.distTag });
		} catch (error) {
			if (
				error?.code === PUBLICATION_FAILURES.AUTHENTICATION_FAILED ||
				error?.code === PUBLICATION_FAILURES.PERMISSION_FAILED
			) {
				throw error;
			}
			publishError = error;
		}
		outcome = PUBLICATION_OUTCOMES.PUBLISHED_AND_VERIFIED;
		try {
			await verifyPublishedRegistryState({
				evidence,
				tarballPath,
				registryClient,
				reconcile,
				verificationAttempts,
				verificationDelayMs,
				sleep,
			});
		} catch (error) {
			if (
				publishError &&
				error?.code === PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED
			) {
				throw publishError;
			}
			throw error;
		}
	}

	await install({
		candidateVersion: evidence.candidateVersion,
		consumerDirectory,
		packageName: evidence.packageName,
		packedFiles: evidence.inventory,
		workspaceDirectory,
	});
	return approvedDiagnostic(evidence, outcome);
}

async function candidateFiles(candidateDirectory) {
	const directory = resolve(candidateDirectory);
	const evidence = JSON.parse(
		await readFile(join(directory, "candidate-evidence.json"), "utf8"),
	);
	const tarballs = (await readdir(directory)).filter((name) => name.endsWith(".tgz"));
	if (tarballs.length !== 1) {
		throw publicationFailure(
			PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED,
			"candidate directory must contain exactly one tarball",
		);
	}
	return { evidence, tarballPath: join(directory, tarballs[0]) };
}

export async function publishCandidateDirectory(
	{ candidateDirectory, workspaceDirectory },
	dependencies = {},
) {
	const loadCandidate = dependencies.candidateFiles ?? candidateFiles;
	const publishAndVerify =
		dependencies.publishAndVerifyCandidate ?? publishAndVerifyCandidate;
	const registryClient =
		dependencies.registryClient ?? createNpmRegistryClient();
	const candidate = await loadCandidate(candidateDirectory);
	const consumerDirectory = await mkdtemp(join(tmpdir(), "you-api-registry-consumer-"));
	try {
		return await publishAndVerify({
			...candidate,
			consumerDirectory,
			registryClient,
			workspaceDirectory,
		});
	} finally {
		await rm(consumerDirectory, { recursive: true, force: true });
	}
}

async function main() {
	const { values } = parseArgs({
		options: {
			"candidate-directory": { type: "string" },
			"workspace-directory": { type: "string" },
		},
		strict: true,
	});
	const result = await publishCandidateDirectory({
		candidateDirectory: values["candidate-directory"],
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
