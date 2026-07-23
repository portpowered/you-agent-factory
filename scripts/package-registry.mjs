import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

export const RECONCILIATION_OUTCOMES = Object.freeze({
	PUBLISH_REQUIRED: "PUBLISH_REQUIRED",
	VERIFIED_EXISTING: "VERIFIED_EXISTING",
});

export const RECONCILIATION_FAILURES = Object.freeze({
	CANDIDATE_INVALID: "CANDIDATE_INVALID",
	CANDIDATE_DIGEST_VERIFICATION_FAILED: "CANDIDATE_DIGEST_VERIFICATION_FAILED",
	REGISTRY_LOOKUP_FAILED: "REGISTRY_LOOKUP_FAILED",
	REGISTRY_DOWNLOAD_FAILED: "REGISTRY_DOWNLOAD_FAILED",
	REGISTRY_AUTHENTICATION_FAILED: "REGISTRY_AUTHENTICATION_FAILED",
	REGISTRY_PERMISSION_FAILED: "REGISTRY_PERMISSION_FAILED",
	REGISTRY_TIMEOUT: "REGISTRY_TIMEOUT",
	IMMUTABLE_VERSION_CONFLICT: "IMMUTABLE_VERSION_CONFLICT",
});

const SHA256_PATTERN = /^sha256:[0-9a-f]{64}$/;
const SOURCE_COMMIT_PATTERN = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
const CANDIDATE_VERSION_PATTERN =
	/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-dev\.([1-9]\d*)\.([0-9a-f]{12})$/;
const DEFAULT_REGISTRY_TIMEOUT_MS = 30_000;

export class RegistryReconciliationError extends Error {
	constructor(code, message, options = {}) {
		super(`[package-registry] ${message}`, options);
		this.name = "RegistryReconciliationError";
		this.code = code;
	}
}

function failure(code, message, cause) {
	return new RegistryReconciliationError(code, message, { cause });
}

function requireCandidateEvidence(
	evidence,
	expectedPackageName,
	expectedDistTag,
) {
	if (!evidence || typeof evidence !== "object" || Array.isArray(evidence)) {
		throw failure(
			RECONCILIATION_FAILURES.CANDIDATE_INVALID,
			"candidate evidence must be an object",
		);
	}
	const versionMatch = CANDIDATE_VERSION_PATTERN.exec(
		evidence.candidateVersion,
	);
	if (
		evidence.packageName !== expectedPackageName ||
		!versionMatch ||
		!SOURCE_COMMIT_PATTERN.test(evidence.sourceCommit ?? "") ||
		versionMatch[5] !== evidence.sourceCommit.slice(0, 12) ||
		!SHA256_PATTERN.test(evidence.contractDigest ?? "") ||
		!SHA256_PATTERN.test(evidence.artifactDigest ?? "") ||
		!Array.isArray(evidence.inventory) ||
		evidence.inventory.some((path) => typeof path !== "string") ||
		evidence.distTag !== expectedDistTag
	) {
		throw failure(
			RECONCILIATION_FAILURES.CANDIDATE_INVALID,
			"candidate evidence has invalid or inconsistent identity fields",
		);
	}
	return evidence;
}

function sha256(contents) {
	return `sha256:${createHash("sha256").update(contents).digest("hex")}`;
}

function diagnostic(evidence, outcome) {
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

function dependencyFailure(error, fallbackCode, fallbackMessage) {
	if (error instanceof RegistryReconciliationError) {
		return error;
	}
	return failure(fallbackCode, fallbackMessage, error);
}

export async function reconcileCandidate({
	evidence,
	expectedDistTag,
	expectedPackageName,
	tarballPath,
	registryClient,
}) {
	const candidate = requireCandidateEvidence(
		evidence,
		expectedPackageName,
		expectedDistTag,
	);
	if (
		!registryClient ||
		typeof registryClient.lookupVersion !== "function" ||
		typeof registryClient.downloadTarball !== "function"
	) {
		throw failure(
			RECONCILIATION_FAILURES.CANDIDATE_INVALID,
			"registry client must provide lookupVersion and downloadTarball",
		);
	}

	let localContents;
	try {
		localContents = await readFile(resolve(tarballPath));
	} catch (error) {
		throw failure(
			RECONCILIATION_FAILURES.CANDIDATE_DIGEST_VERIFICATION_FAILED,
			"candidate tarball could not be read for digest verification",
			error,
		);
	}
	if (sha256(localContents) !== candidate.artifactDigest) {
		throw failure(
			RECONCILIATION_FAILURES.CANDIDATE_DIGEST_VERIFICATION_FAILED,
			"candidate tarball digest does not match candidate evidence",
		);
	}

	let registryVersion;
	try {
		registryVersion = await registryClient.lookupVersion({
			packageName: candidate.packageName,
			version: candidate.candidateVersion,
		});
	} catch (error) {
		throw dependencyFailure(
			error,
			RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED,
			"registry version lookup failed",
		);
	}
	if (registryVersion?.status === "absent") {
		return diagnostic(candidate, RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED);
	}
	if (
		registryVersion?.status !== "present" ||
		typeof registryVersion.tarballUrl !== "string" ||
		registryVersion.tarballUrl.length === 0
	) {
		throw failure(
			RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED,
			"registry version lookup returned an invalid response",
		);
	}

	let registryContents;
	try {
		registryContents = await registryClient.downloadTarball(
			registryVersion.tarballUrl,
		);
	} catch (error) {
		throw dependencyFailure(
			error,
			RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED,
			"registry tarball download failed",
		);
	}
	if (sha256(registryContents) !== candidate.artifactDigest) {
		throw failure(
			RECONCILIATION_FAILURES.IMMUTABLE_VERSION_CONFLICT,
			"existing registry version has a different immutable tarball digest",
		);
	}

	return diagnostic(candidate, RECONCILIATION_OUTCOMES.VERIFIED_EXISTING);
}

function classifyNpmFailure(output) {
	if (/\b(?:E401|ENEEDAUTH)\b|\b401\b/.test(output)) {
		return failure(
			RECONCILIATION_FAILURES.REGISTRY_AUTHENTICATION_FAILED,
			"registry authentication failed",
		);
	}
	if (/\bE403\b|\b403\b/.test(output)) {
		return failure(
			RECONCILIATION_FAILURES.REGISTRY_PERMISSION_FAILED,
			"registry permission denied",
		);
	}
	if (/\bE404\b|\b404\b/.test(output)) {
		return { status: "absent" };
	}
	return failure(
		RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED,
		"registry version lookup failed",
	);
}

function runNpmView(packageName, version, timeoutMs) {
	return new Promise((resolvePromise, rejectPromise) => {
		const child = spawn(
			"npm",
			["view", `${packageName}@${version}`, "dist.tarball", "--json"],
			{
				shell: process.platform === "win32",
				stdio: ["ignore", "pipe", "pipe"],
			},
		);
		let stdout = "";
		let stderr = "";
		let timedOut = false;
		const timeout = setTimeout(() => {
			timedOut = true;
			child.kill();
		}, timeoutMs);
		child.stdout.setEncoding("utf8");
		child.stderr.setEncoding("utf8");
		child.stdout.on("data", (chunk) => {
			stdout += chunk;
		});
		child.stderr.on("data", (chunk) => {
			stderr += chunk;
		});
		child.on("error", (error) => {
			clearTimeout(timeout);
			rejectPromise(error);
		});
		child.on("close", (status) => {
			clearTimeout(timeout);
			if (timedOut) {
				rejectPromise(
					failure(
						RECONCILIATION_FAILURES.REGISTRY_TIMEOUT,
						"registry version lookup timed out",
					),
				);
				return;
			}
			if (status !== 0) {
				resolvePromise(classifyNpmFailure(`${stdout}\n${stderr}`));
				return;
			}
			try {
				const tarballUrl = JSON.parse(stdout);
				if (typeof tarballUrl !== "string" || tarballUrl.length === 0) {
					throw new Error("invalid tarball URL");
				}
				resolvePromise({ status: "present", tarballUrl });
			} catch (error) {
				rejectPromise(
					failure(
						RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED,
						"registry version lookup returned invalid JSON",
						error,
					),
				);
			}
		});
	});
}

function downloadRegistryTarball(url, timeoutMs) {
	const controller = new AbortController();
	const timeout = setTimeout(() => controller.abort(), timeoutMs);
	return fetch(url, { signal: controller.signal })
		.then(async (response) => {
			if (response.status === 401) {
				throw failure(
					RECONCILIATION_FAILURES.REGISTRY_AUTHENTICATION_FAILED,
					"registry authentication failed while downloading tarball",
				);
			}
			if (response.status === 403) {
				throw failure(
					RECONCILIATION_FAILURES.REGISTRY_PERMISSION_FAILED,
					"registry permission denied while downloading tarball",
				);
			}
			if (!response.ok) {
				throw failure(
					RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED,
					"registry tarball download failed",
				);
			}
			return new Uint8Array(await response.arrayBuffer());
		})
		.catch((error) => {
			if (error?.name === "AbortError") {
				throw failure(
					RECONCILIATION_FAILURES.REGISTRY_TIMEOUT,
					"registry tarball download timed out",
					error,
				);
			}
			throw dependencyFailure(
				error,
				RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED,
				"registry tarball download failed",
			);
		})
		.finally(() => clearTimeout(timeout));
}

export function createNpmRegistryClient({
	timeoutMs = DEFAULT_REGISTRY_TIMEOUT_MS,
} = {}) {
	return {
		async lookupVersion({ packageName, version }) {
			const result = await runNpmView(packageName, version, timeoutMs);
			if (result instanceof RegistryReconciliationError) {
				throw result;
			}
			return result;
		},
		downloadTarball(url) {
			return downloadRegistryTarball(url, timeoutMs);
		},
	};
}
