import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
	mkdir,
	mkdtemp,
	readFile,
	rm,
	unlink,
	writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
	FRONTEND_ONLY_CANDIDATE_SCOPE,
	FRONTEND_PUBLIC_PACKAGE_NAMES,
	TAGGED_RELEASE_CANDIDATE_SCOPE,
	TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES,
} from "./public-package-set.mjs";
import { publishTaggedReleaseCandidate } from "./public-release-package-publish.mjs";
import { smokeFrontendPackages } from "./public-package-install-smoke.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";
const version = "1.2.3";

async function writeJson(path, value) {
	await writeFile(path, `${JSON.stringify(value, null, 2)}\n`);
}

function digests(contents) {
	return {
		artifactDigest: `sha256:${createHash("sha256").update(contents).digest("hex")}`,
		integrity: `sha512-${createHash("sha512").update(contents).digest("base64")}`,
		shasum: createHash("sha1").update(contents).digest("hex"),
	};
}

async function candidateFixture(t) {
	const root = await mkdtemp(join(tmpdir(), "you-tagged-publisher-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	await Promise.all(
		["api", "packaged-factories", "frontend"].map((directory) =>
			mkdir(join(root, directory)),
		),
	);
	const apiTarball = "you-agent-factory-api-1.2.3.tgz";
	const factoriesTarball = "you-agent-factory-packaged-factories-1.2.3.tgz";
	const apiContents = "api";
	const factoriesContents = "factories";
	await Promise.all([
		writeFile(join(root, "api", apiTarball), apiContents),
		writeFile(
			join(root, "packaged-factories", factoriesTarball),
			factoriesContents,
		),
		writeJson(join(root, "api", "candidate-evidence.json"), {
			packageName: "@you-agent-factory/api",
			candidateVersion: version,
			sourceCommit,
			distTag: "latest",
			contractDigest: digests("api-contract").artifactDigest,
			artifactDigest: digests(apiContents).artifactDigest,
			inventory: ["package.json"],
		}),
		writeJson(join(root, "packaged-factories", "candidate-evidence.json"), {
			packageName: "@you-agent-factory/packaged-factories",
			candidateVersion: version,
			sourceCommit,
			distTag: "latest",
			contractDigest: digests("factories-contract").artifactDigest,
			artifactDigest: digests(factoriesContents).artifactDigest,
			inventory: ["package.json"],
		}),
	]);
	const frontendPackages = FRONTEND_PUBLIC_PACKAGE_NAMES.map((name, index) => {
		const filename = `frontend-${index}.tgz`;
		return { name, version, filename, ...digests(filename) };
	});
	await Promise.all(
		frontendPackages.map(({ filename }) =>
			writeFile(join(root, "frontend", filename), filename),
		),
	);
	await writeJson(join(root, "frontend", "public-package-candidates.json"), {
		scope: FRONTEND_ONLY_CANDIDATE_SCOPE,
		version,
		packages: frontendPackages,
	});
	const tarballs = new Map([
		["@you-agent-factory/api", `api/${apiTarball}`],
		[
			"@you-agent-factory/packaged-factories",
			`packaged-factories/${factoriesTarball}`,
		],
		...frontendPackages.map(({ name, filename }) => [
			name,
			`frontend/${filename}`,
		]),
	]);
	const evidence = {
		scope: TAGGED_RELEASE_CANDIDATE_SCOPE,
		version,
		sourceCommit,
		packages: TAGGED_RELEASE_PUBLIC_PACKAGE_NAMES.map((name) => ({
			name,
			version,
			tarball: tarballs.get(name),
		})),
	};
	await writeJson(join(root, "release-candidate-evidence.json"), evidence);
	return { evidence, root };
}

function recordingPublishers(calls) {
	return {
		async smoke(input) {
			calls.push(["smoke", input]);
		},
		async publishApiCandidate(input) {
			calls.push(["api", input]);
			return "api-published";
		},
		async publishPackagedFactoriesCandidate(input) {
			calls.push(["packaged-factories", input]);
			return "factories-published";
		},
		async publishFrontendCandidates(input) {
			calls.push(["frontend", input]);
			return "frontend-published";
		},
	};
}

test("validates the complete reviewed set before publishing each represented child candidate", async (t) => {
	const { evidence, root } = await candidateFixture(t);
	const calls = [];
	const result = await publishTaggedReleaseCandidate(
		{
			candidateDirectory: root,
			expectedSourceCommit: sourceCommit,
			workspaceDirectory: "workspace",
		},
		recordingPublishers(calls),
	);

	assert.deepEqual(result.evidence, evidence);
	assert.deepEqual(
		calls.map(([name]) => name),
		["smoke", "api", "packaged-factories", "frontend"],
	);
	assert.equal(calls[0][1].expectedSourceCommit, sourceCommit);
	assert.equal(calls[1][1].expectedDistTag, "latest");
	assert.equal(calls[2][1].expectedDistTag, "latest");
	assert.equal(calls[3][1].tag, "latest");
	assert.equal(calls[3][1].provenance, true);
});

test("an installed operation failure prevents every publication", async (t) => {
	const { root } = await candidateFixture(t);
	const calls = [];
	await assert.rejects(
		publishTaggedReleaseCandidate(
			{
				candidateDirectory: root,
				expectedSourceCommit: sourceCommit,
			},
			{
				...recordingPublishers(calls),
				smoke: async () => {
					throw new Error("installed factory operation failed");
				},
			},
		),
		/installed factory operation failed/,
	);
	assert.deepEqual(calls, []);
});

test("source mismatch prevents installed preflight and publication", async (t) => {
	const { root } = await candidateFixture(t);
	const calls = [];
	await assert.rejects(
		publishTaggedReleaseCandidate(
			{
				candidateDirectory: root,
				expectedSourceCommit: "f".repeat(40),
			},
			recordingPublishers(calls),
		),
		/source commit does not match/,
	);
	assert.deepEqual(calls, []);
});

for (const failure of [null, "install", "operation"]) {
	test(`tagged consumer installs all eight together and cleans up after ${failure ?? "success"}`, async (t) => {
		const { root, evidence } = await candidateFixture(t);
		const calls = [];
		let consumerDirectory;
		const operation = smokeFrontendPackages({
			candidateDirectory: root,
			expectedSourceCommit: sourceCommit,
			log: () => {},
			runCommand: async (command, args, options) => {
				consumerDirectory = options.cwd;
				calls.push(command);
				const manifest = JSON.parse(
					await readFile(join(options.cwd, "package.json"), "utf8"),
				);
				for (const { name } of evidence.packages) {
					assert.ok(manifest.dependencies[name].endsWith(".tgz"));
					assert.equal(manifest.overrides[name], manifest.dependencies[name]);
				}
				const expected = JSON.parse(
					await readFile(join(options.cwd, "expected.json"), "utf8"),
				);
				assert.equal(expected.length, 8);
				assert.ok(
					expected.every(
						(entry) =>
							entry.sourceCommit === sourceCommit && entry.version === version,
					),
				);
				assert.equal(options.env.NODE_PATH, "");
				assert.equal(options.env.NODE_OPTIONS, "");
				if (
					(failure === "install" && command === "bun") ||
					(failure === "operation" && command === "node")
				) {
					throw new Error(`controlled ${failure} failure`);
				}
				return { stdout: "passed", stderr: "" };
			},
		});
		if (failure)
			await assert.rejects(
				operation,
				new RegExp(`controlled ${failure} failure`),
			);
		else assert.deepEqual(await operation, evidence);
		assert.deepEqual(calls, failure === "install" ? ["bun"] : ["bun", "node"]);
		await assert.rejects(readFile(join(consumerDirectory, "package.json")), {
			code: "ENOENT",
		});
	});
}

for (const testCase of [
	{
		name: "unknown scope",
		mutate(evidence) {
			evidence.scope = "complete";
		},
		expected: /unknown candidate scope/,
	},
	{
		name: "duplicate package",
		mutate(evidence) {
			evidence.packages[1].name = evidence.packages[0].name;
		},
		expected: /duplicate package names/,
	},
	{
		name: "missing expected package",
		mutate(evidence) {
			evidence.packages.pop();
		},
		expected: /candidate count mismatch/,
	},
	{
		name: "mismatched candidate count",
		mutate(evidence) {
			evidence.packages.push({
				name: "@you-agent-factory/unreviewed",
				version,
				tarball: "frontend/unreviewed.tgz",
			});
		},
		expected: /candidate count mismatch/,
	},
	{
		name: "unrepresented child tarball",
		mutate(evidence) {
			evidence.packages[0].tarball = "api/different.tgz";
		},
		expected: /top-level evidence does not represent/,
	},
]) {
	test(`rejects ${testCase.name} before any package publication`, async (t) => {
		const { evidence, root } = await candidateFixture(t);
		testCase.mutate(evidence);
		await writeJson(join(root, "release-candidate-evidence.json"), evidence);
		const calls = [];

		await assert.rejects(
			publishTaggedReleaseCandidate(
				{
					candidateDirectory: root,
					expectedSourceCommit: sourceCommit,
					workspaceDirectory: "workspace",
				},
				recordingPublishers(calls),
			),
			testCase.expected,
		);
		assert.deepEqual(calls, []);
	});
}

for (const field of ["shasum", "integrity"]) {
	test(`mismatched frontend ${field} prevents every publication side effect`, async (t) => {
		const { root } = await candidateFixture(t);
		const evidencePath = join(
			root,
			"frontend",
			"public-package-candidates.json",
		);
		const child = JSON.parse(await readFile(evidencePath, "utf8"));
		child.packages[0][field] =
			field === "shasum" ? "0".repeat(40) : `sha512-${"A".repeat(88)}`;
		await writeJson(evidencePath, child);
		const calls = [];

		await assert.rejects(
			publishTaggedReleaseCandidate(
				{
					candidateDirectory: root,
					expectedSourceCommit: sourceCommit,
					workspaceDirectory: "workspace",
				},
				recordingPublishers(calls),
			),
			new RegExp(`candidate ${field} mismatch`),
		);
		assert.deepEqual(calls, []);
	});
}

for (const testCase of [
	{
		child: "api",
		action: "corrupt",
		tarball: "you-agent-factory-api-1.2.3.tgz",
	},
	{
		child: "api",
		action: "remove",
		tarball: "you-agent-factory-api-1.2.3.tgz",
	},
	{
		child: "packaged-factories",
		action: "corrupt",
		tarball: "you-agent-factory-packaged-factories-1.2.3.tgz",
	},
	{
		child: "packaged-factories",
		action: "remove",
		tarball: "you-agent-factory-packaged-factories-1.2.3.tgz",
	},
	{ child: "frontend", action: "corrupt", tarball: "frontend-0.tgz" },
	{ child: "frontend", action: "remove", tarball: "frontend-0.tgz" },
]) {
	test(`${testCase.action === "remove" ? "missing" : "corrupt"} ${testCase.child} tarball prevents every publication side effect`, async (t) => {
		const { root } = await candidateFixture(t);
		const tarballPath = join(root, testCase.child, testCase.tarball);
		if (testCase.action === "remove") await unlink(tarballPath);
		else await writeFile(tarballPath, "tampered");
		const calls = [];

		await assert.rejects(
			publishTaggedReleaseCandidate(
				{
					candidateDirectory: root,
					expectedSourceCommit: sourceCommit,
					workspaceDirectory: "workspace",
				},
				recordingPublishers(calls),
			),
			/candidate (?:digest|shasum|integrity) mismatch|represented tarball is not readable|exactly one tarball/,
		);
		assert.deepEqual(calls, []);
	});
}
