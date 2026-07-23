import { spawn } from "node:child_process";
import { access } from "node:fs/promises";
import { join, resolve } from "node:path";

export const REVIEWED_PACK_FILES = Object.freeze([
	"README.md",
	"LICENSE.md",
	"generated/manifest.json",
	"generated/openapi/openapi.yaml",
	"generated/cli/commands.json",
	"generated/mcp/tools.json",
	"generated/schemas/you-config.schema.json",
	"generated/schemas/factory.schema.json",
	"generated/schemas/factory-event.schema.json",
	"generated/schemas/factory-recording.schema.json",
	"generated/schemas/mock-workers.schema.json",
	"generated/javascript/runtime-api.json",
	"generated/joined/contracts/common/deprecations.schema.json",
	"generated/joined/contracts/common/documentation.schema.json",
	"generated/joined/contracts/manifest.schema.json",
	"package.json",
]);

function normalizedPath(path) {
	return path.replaceAll("\\", "/").replace(/^package\//, "");
}

function sortedUnique(paths) {
	return [...new Set(paths.map(normalizedPath))].sort((left, right) =>
		left.localeCompare(right),
	);
}

export function inventoryDrift(actualFiles) {
	const actual = sortedUnique(actualFiles);
	const expected = sortedUnique(REVIEWED_PACK_FILES);
	const actualSet = new Set(actual);
	const expectedSet = new Set(expected);

	return {
		unexpected: actual.filter((path) => !expectedSet.has(path)),
		missing: expected.filter((path) => !actualSet.has(path)),
	};
}

export function assertReviewedInventory(actualFiles) {
	const drift = inventoryDrift(actualFiles);
	if (drift.unexpected.length === 0 && drift.missing.length === 0) {
		return;
	}

	const diagnostics = ["[api-package-pack] tarball inventory rejected"];
	if (drift.unexpected.length > 0) {
		diagnostics.push(
			"unexpected package files:",
			...drift.unexpected.map((path) => `  ${path}`),
		);
	}
	if (drift.missing.length > 0) {
		diagnostics.push(
			"missing package files:",
			...drift.missing.map((path) => `  ${path}`),
		);
	}
	throw new Error(diagnostics.join("\n"));
}

function runNpmPack(packageDirectory, packDestination) {
	const arguments_ = [
		"pack",
		"--json",
		"--ignore-scripts",
		"--pack-destination",
		packDestination,
		packageDirectory,
	];

	return new Promise((resolvePromise, rejectPromise) => {
		const child = spawn("npm", arguments_, {
			shell: process.platform === "win32",
			stdio: ["ignore", "pipe", "pipe"],
		});
		let stdout = "";
		let stderr = "";
		child.stdout.setEncoding("utf8");
		child.stderr.setEncoding("utf8");
		child.stdout.on("data", (chunk) => {
			stdout += chunk;
		});
		child.stderr.on("data", (chunk) => {
			stderr += chunk;
		});
		child.on("error", rejectPromise);
		child.on("close", (status) => {
			if (status !== 0) {
				rejectPromise(
					new Error(
						`[api-package-pack] npm pack failed with status ${status}\n${stderr.trim()}`,
					),
				);
				return;
			}
			resolvePromise(stdout);
		});
	});
}

export async function packPackage({ packageDirectory, packDestination }) {
	const packageRoot = resolve(packageDirectory);
	const destination = resolve(packDestination);
	const stdout = await runNpmPack(packageRoot, destination);

	let reports;
	try {
		reports = JSON.parse(stdout);
	} catch (error) {
		throw new Error("[api-package-pack] npm pack did not return JSON", {
			cause: error,
		});
	}
	if (!Array.isArray(reports) || reports.length !== 1) {
		throw new Error(
			`[api-package-pack] npm pack returned ${reports?.length ?? "invalid"} reports, want 1`,
		);
	}

	const report = reports[0];
	if (typeof report.name !== "string" || report.name.length === 0) {
		throw new Error(
			"[api-package-pack] npm pack report has no valid package name",
		);
	}
	if (typeof report.version !== "string" || report.version.length === 0) {
		throw new Error(
			"[api-package-pack] npm pack report has no valid package version",
		);
	}
	const files = report.files?.map((file) => file.path);
	if (!Array.isArray(files) || files.some((path) => typeof path !== "string")) {
		throw new Error(
			"[api-package-pack] npm pack report has no valid file inventory",
		);
	}
	const tarballPath = join(destination, report.filename);
	await access(tarballPath);
	return {
		files: sortedUnique(files),
		packageName: report.name,
		packageVersion: report.version,
		tarballPath,
	};
}

export async function packAndVerify(input) {
	const packed = await packPackage(input);
	assertReviewedInventory(packed.files);
	return packed;
}
