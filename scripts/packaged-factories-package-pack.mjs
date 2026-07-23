import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { access, lstat, readFile, realpath } from "node:fs/promises";
import { isAbsolute, join, relative, resolve } from "node:path";

const DIAGNOSTIC_PREFIX = "[packaged-factories-package-pack]";
const STATIC_PACK_FILES = Object.freeze([
	"LICENSE.md",
	"README.md",
	"generated/README.md",
	"generated/manifest.json",
	"package.json",
	"schemas/factory.schema.json",
	"schemas/factory.schema.yaml",
]);
const PUBLIC_EXPORTS = Object.freeze({
	"./manifest": "./generated/manifest.json",
	"./schemas/factory.json": "./schemas/factory.schema.json",
	"./schemas/factory.yaml": "./schemas/factory.schema.yaml",
	"./factories/*.json": "./generated/factories/*/factory.json",
	"./factories/*.yaml": "./generated/factories/*/factory.yaml",
});
const FILE_REFERENCE_KEYS = new Set([
	"$ref",
	"file",
	"locator",
	"path",
	"source",
]);

function normalizedPath(path) {
	return path.replaceAll("\\", "/").replace(/^package\//, "");
}

function sortedUnique(paths) {
	return [...new Set(paths.map(normalizedPath))].sort((left, right) =>
		left.localeCompare(right),
	);
}

async function readJSON(path, description) {
	try {
		return JSON.parse(await readFile(path, "utf8"));
	} catch (error) {
		throw new Error(
			`${DIAGNOSTIC_PREFIX} ${description} is not readable JSON`,
			{
				cause: error,
			},
		);
	}
}

function requirePackagePath(path, description) {
	if (
		typeof path !== "string" ||
		path.length === 0 ||
		path.includes("\\") ||
		isAbsolute(path) ||
		path.startsWith("/") ||
		path.split("/").some((part) => part === "" || part === "." || part === "..")
	) {
		throw new Error(
			`${DIAGNOSTIC_PREFIX} ${description} escapes the package: ${JSON.stringify(path)}`,
		);
	}
	return path;
}

function artifactLocators(contractManifest) {
	if (
		!Array.isArray(contractManifest.factories) ||
		contractManifest.factories.length === 0
	) {
		throw new Error(`${DIAGNOSTIC_PREFIX} contract manifest has no factories`);
	}
	return contractManifest.factories.flatMap((factory, index) => {
		const slug = requirePackagePath(
			factory?.slug,
			`manifest factories[${index}] slug`,
		);
		const expected = [
			`generated/factories/${slug}/factory.json`,
			`generated/factories/${slug}/factory.yaml`,
		];
		const actual = [
			requirePackagePath(
				factory?.json?.locator,
				`manifest factories[${index}] JSON locator`,
			),
			requirePackagePath(
				factory?.yaml?.locator,
				`manifest factories[${index}] YAML locator`,
			),
		];
		for (let format = 0; format < expected.length; format += 1) {
			if (actual[format] !== expected[format]) {
				throw new Error(
					`${DIAGNOSTIC_PREFIX} manifest locator ${JSON.stringify(actual[format])} does not match ${JSON.stringify(expected[format])}`,
				);
			}
		}
		return actual;
	});
}

function assertPublicExports(packageManifest) {
	const actual = packageManifest.exports;
	const expectedKeys = Object.keys(PUBLIC_EXPORTS);
	if (
		typeof actual !== "object" ||
		actual === null ||
		Array.isArray(actual) ||
		Object.keys(actual).length !== expectedKeys.length
	) {
		throw new Error(
			`${DIAGNOSTIC_PREFIX} package exports must contain exactly the reviewed public data specifiers`,
		);
	}
	for (const specifier of expectedKeys) {
		const target = actual[specifier];
		if (target !== PUBLIC_EXPORTS[specifier]) {
			throw new Error(
				`${DIAGNOSTIC_PREFIX} export ${specifier} must target ${PUBLIC_EXPORTS[specifier]}, got ${JSON.stringify(target)}`,
			);
		}
		requirePackagePath(target.replace(/^\.\//, ""), `export ${specifier}`);
	}
}

export async function reviewedPackFiles(packageDirectory) {
	const packageRoot = resolve(packageDirectory);
	const packageManifest = await readJSON(
		join(packageRoot, "package.json"),
		"package manifest",
	);
	const contractManifest = await readJSON(
		join(packageRoot, "generated", "manifest.json"),
		"contract manifest",
	);
	assertPublicExports(packageManifest);
	return sortedUnique([
		...STATIC_PACK_FILES,
		...artifactLocators(contractManifest),
	]);
}

export function inventoryDrift(actualFiles, expectedFiles) {
	const actual = sortedUnique(actualFiles);
	const expected = sortedUnique(expectedFiles);
	const actualSet = new Set(actual);
	const expectedSet = new Set(expected);
	return {
		unexpected: actual.filter((path) => !expectedSet.has(path)),
		missing: expected.filter((path) => !actualSet.has(path)),
	};
}

export function assertReviewedInventory(actualFiles, expectedFiles) {
	const drift = inventoryDrift(actualFiles, expectedFiles);
	if (drift.unexpected.length === 0 && drift.missing.length === 0) {
		return;
	}
	const diagnostics = [`${DIAGNOSTIC_PREFIX} tarball inventory rejected`];
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

export const npmPackArguments = (packageDirectory, packDestination) => [
	"pack",
	"--json",
	"--ignore-scripts",
	"--pack-destination",
	packDestination,
	packageDirectory,
];

export function runNpmPack(packageDirectory, packDestination) {
	const arguments_ = npmPackArguments(packageDirectory, packDestination);
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
						`${DIAGNOSTIC_PREFIX} npm pack failed with status ${status}\n${stderr.trim()}`,
					),
				);
				return;
			}
			resolvePromise(stdout);
		});
	});
}

function parsePackReport(stdout) {
	let reports;
	try {
		reports = JSON.parse(stdout);
	} catch (error) {
		throw new Error(`${DIAGNOSTIC_PREFIX} npm pack did not return JSON`, {
			cause: error,
		});
	}
	if (!Array.isArray(reports) || reports.length !== 1) {
		throw new Error(
			`${DIAGNOSTIC_PREFIX} npm pack returned ${reports?.length ?? "invalid"} reports, want 1`,
		);
	}
	const report = reports[0];
	const files = report.files?.map((file) => file.path);
	if (
		typeof report.name !== "string" ||
		typeof report.version !== "string" ||
		typeof report.filename !== "string" ||
		typeof report.shasum !== "string" ||
		!Array.isArray(files) ||
		files.some((path) => typeof path !== "string")
	) {
		throw new Error(`${DIAGNOSTIC_PREFIX} npm pack returned an invalid report`);
	}
	return { ...report, files };
}

function containedPath(root, target) {
	const path = relative(root, target);
	return path === "" || (!path.startsWith("..") && !isAbsolute(path));
}

async function assertRegularContainedFiles(packageRoot, paths) {
	const canonicalRoot = await realpath(packageRoot);
	for (const path of paths) {
		const target = join(packageRoot, ...path.split("/"));
		const metadata = await lstat(target);
		if (!metadata.isFile() || metadata.isSymbolicLink()) {
			throw new Error(
				`${DIAGNOSTIC_PREFIX} package file must be a regular non-symlink: ${path}`,
			);
		}
		const canonicalTarget = await realpath(target);
		if (!containedPath(canonicalRoot, canonicalTarget)) {
			throw new Error(
				`${DIAGNOSTIC_PREFIX} package file resolves outside the package: ${path}`,
			);
		}
	}
}

async function assertNoSymlinkFiles(packageRoot, paths) {
	for (const path of paths) {
		try {
			const metadata = await lstat(join(packageRoot, ...path.split("/")));
			if (metadata.isSymbolicLink()) {
				throw new Error(
					`${DIAGNOSTIC_PREFIX} package file must not be a symlink: ${path}`,
				);
			}
		} catch (error) {
			if (error?.code !== "ENOENT") {
				throw error;
			}
		}
	}
}

function externalFileReference(value) {
	if (
		typeof value !== "string" ||
		value.length === 0 ||
		value.startsWith("#")
	) {
		return false;
	}
	return (
		value.startsWith("file:") ||
		value.startsWith("./") ||
		value.startsWith("../") ||
		value.startsWith("/") ||
		/^[A-Za-z]:[\\/]/.test(value)
	);
}

function findExternalDependencies(value, location = "$", findings = []) {
	if (Array.isArray(value)) {
		value.forEach((entry, index) => {
			findExternalDependencies(entry, `${location}[${index}]`, findings);
		});
		return findings;
	}
	if (typeof value !== "object" || value === null) {
		return findings;
	}
	for (const [key, child] of Object.entries(value)) {
		const childLocation = `${location}.${key}`;
		if (FILE_REFERENCE_KEYS.has(key) && externalFileReference(child)) {
			findings.push(`${childLocation}=${JSON.stringify(child)}`);
		}
		findExternalDependencies(child, childLocation, findings);
	}
	return findings;
}

async function assertSelfContainedArtifacts(packageRoot, artifactFiles) {
	for (const path of artifactFiles.filter((path) => path.endsWith(".json"))) {
		const document = await readJSON(
			join(packageRoot, ...path.split("/")),
			path,
		);
		const findings = findExternalDependencies(document);
		if (findings.length > 0) {
			throw new Error(
				`${DIAGNOSTIC_PREFIX} flattened artifact has package-external file dependencies:\n${findings.map((finding) => `  ${path}: ${finding}`).join("\n")}`,
			);
		}
	}
}

export function verifyTarballDigest(contents, expectedShasum) {
	const actual = createHash("sha1").update(contents).digest("hex");
	if (actual !== expectedShasum) {
		throw new Error(
			`${DIAGNOSTIC_PREFIX} tarball digest mismatch: npm=${expectedShasum} actual=${actual}`,
		);
	}
}

export async function packAndVerify({
	packageDirectory,
	packDestination,
	npmPack = runNpmPack,
}) {
	const packageRoot = resolve(packageDirectory);
	const destination = resolve(packDestination);
	const expectedFiles = await reviewedPackFiles(packageRoot);
	await assertNoSymlinkFiles(packageRoot, expectedFiles);
	const stdout = await npmPack(packageRoot, destination);
	const report = parsePackReport(stdout);
	assertReviewedInventory(report.files, expectedFiles);
	await assertRegularContainedFiles(packageRoot, expectedFiles);
	const artifactFiles = expectedFiles.filter((path) =>
		path.startsWith("generated/factories/"),
	);
	await assertSelfContainedArtifacts(packageRoot, artifactFiles);
	const tarballPath = join(destination, report.filename);
	await access(tarballPath);
	verifyTarballDigest(await readFile(tarballPath), report.shasum);
	return {
		files: sortedUnique(report.files),
		packageName: report.name,
		packageVersion: report.version,
		tarballPath,
	};
}

export function runCatalogDriftCheck(repositoryRoot) {
	return new Promise((resolvePromise, rejectPromise) => {
		const child = spawn(
			"go",
			["run", "./cmd/packagedfactorycatalogcheck", "-root", repositoryRoot],
			{
				cwd: repositoryRoot,
				shell: process.platform === "win32",
				stdio: ["ignore", "pipe", "pipe"],
			},
		);
		let stderr = "";
		child.stderr.setEncoding("utf8");
		child.stderr.on("data", (chunk) => {
			stderr += chunk;
		});
		child.on("error", rejectPromise);
		child.on("close", (status) => {
			if (status !== 0) {
				rejectPromise(
					new Error(
						`${DIAGNOSTIC_PREFIX} generated catalog drift check failed\n${stderr.trim()}`,
					),
				);
				return;
			}
			resolvePromise();
		});
	});
}
