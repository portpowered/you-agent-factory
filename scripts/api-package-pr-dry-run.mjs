import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";

import { prepareCandidate } from "./api-package-candidate.mjs";
import { installAndVerifyTarball } from "./api-package-consumer.mjs";

export const PULL_REQUEST_EVENT = "pull_request";
export const DRY_RUN_OUTCOME = "DRY_RUN_NO_PUBLISH";

function requireString(value, name) {
	if (typeof value !== "string" || value.length === 0) {
		throw new Error(`[api-package-pr-dry-run] ${name} is required`);
	}
	return value;
}

export async function validatePullRequestCandidate(
	{
		eventName,
		outputDirectory,
		packageDirectory,
		runId,
		sourceCommit,
		workspaceDirectory,
	},
	dependencies = {},
) {
	if (eventName !== PULL_REQUEST_EVENT) {
		throw new Error(
			"[api-package-pr-dry-run] only pull_request events may use the dry-run path",
		);
	}

	const prepare = dependencies.prepareCandidate ?? prepareCandidate;
	const verify = dependencies.installAndVerifyTarball ?? installAndVerifyTarball;
	const workspaceRoot = resolve(
		requireString(workspaceDirectory, "workspace directory"),
	);
	const prepared = await prepare({
		packageDirectory: requireString(packageDirectory, "package directory"),
		outputDirectory: requireString(outputDirectory, "output directory"),
		runId: requireString(runId, "run ID"),
		sourceCommit: requireString(sourceCommit, "source commit"),
	});

	const consumerDirectory = await mkdtemp(
		join(tmpdir(), "you-api-pr-candidate-consumer-"),
	);
	try {
		await writeFile(
			join(consumerDirectory, "package.json"),
			'{"name":"api-pr-candidate-consumer","private":true}\n',
		);
		await verify({
			consumerDirectory,
			packageName: prepared.evidence.packageName,
			packedFiles: prepared.evidence.inventory,
			tarballPath: prepared.tarballPath,
			workspaceDirectory: workspaceRoot,
		});
	} finally {
		await rm(consumerDirectory, { recursive: true, force: true });
	}

	return { ...prepared.evidence, outcome: DRY_RUN_OUTCOME };
}

async function main() {
	const { values } = parseArgs({
		options: {
			"event-name": { type: "string" },
			"output-directory": { type: "string" },
			"package-directory": { type: "string" },
			"run-id": { type: "string" },
			"source-commit": { type: "string" },
			"workspace-directory": { type: "string" },
		},
		strict: true,
	});
	const result = await validatePullRequestCandidate({
		eventName: values["event-name"],
		outputDirectory: values["output-directory"],
		packageDirectory: values["package-directory"],
		runId: values["run-id"],
		sourceCommit: values["source-commit"],
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
