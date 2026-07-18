import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { lstat, readFile, realpath } from "node:fs/promises";
import { createRequire } from "node:module";
import { isAbsolute, join, relative, resolve, sep } from "node:path";

function normalizedPath(path) {
	return path.replaceAll("\\", "/").replace(/^\.\//, "");
}

function isWithin(parent, candidate) {
	const child = relative(parent, candidate);
	return (
		child === "" ||
		(!child.startsWith(`..${sep}`) && child !== ".." && !isAbsolute(child))
	);
}

function substituteWildcard(pattern, value) {
	return pattern.replace("*", value);
}

export function concreteExportCases(packageManifest, packedFiles) {
	const packageName = packageManifest.name;
	const files = [...new Set(packedFiles.map(normalizedPath))].sort(
		(left, right) => left.localeCompare(right),
	);
	const cases = [];
	const missingNamedTargets = [];

	for (const [publicKey, rawTarget] of Object.entries(
		packageManifest.exports,
	)) {
		const target = normalizedPath(rawTarget);
		const publicWildcard = publicKey.indexOf("*");
		const targetWildcard = target.indexOf("*");

		if (publicWildcard === -1 && targetWildcard === -1) {
			if (!files.includes(target)) {
				missingNamedTargets.push(`${publicKey} -> ${target}`);
				continue;
			}
			cases.push({
				specifier: `${packageName}${publicKey.slice(1)}`,
				target,
			});
			continue;
		}
		if (publicWildcard === -1 || targetWildcard === -1) {
			throw new Error(
				`[api-package-consumer] wildcard mismatch for export ${publicKey}`,
			);
		}

		const targetPrefix = target.slice(0, targetWildcard);
		const targetSuffix = target.slice(targetWildcard + 1);
		for (const file of files) {
			if (!file.startsWith(targetPrefix) || !file.endsWith(targetSuffix)) {
				continue;
			}
			const match = file.slice(
				targetPrefix.length,
				targetSuffix.length === 0 ? undefined : -targetSuffix.length,
			);
			if (match.length === 0) {
				continue;
			}
			cases.push({
				specifier: `${packageName}${substituteWildcard(publicKey, match).slice(1)}`,
				target: file,
			});
		}
	}
	if (missingNamedTargets.length > 0) {
		throw new Error(
			[
				"[api-package-consumer] missing named export targets:",
				...missingNamedTargets.sort().map((entry) => `  ${entry}`),
			].join("\n"),
		);
	}

	return cases.sort((left, right) =>
		left.specifier.localeCompare(right.specifier),
	);
}

export async function verifyResolvedExport({
	packageRoot,
	resolveSpecifier,
	specifier,
	target,
}) {
	let resolvedPath;
	try {
		resolvedPath = await resolveSpecifier(specifier);
	} catch (error) {
		throw new Error(
			`[api-package-consumer] export did not resolve: ${specifier}`,
			{
				cause: error,
			},
		);
	}

	const canonicalPackageRoot = await realpath(packageRoot);
	const canonicalTarget = await realpath(resolvedPath).catch((error) => {
		throw new Error(
			`[api-package-consumer] export target is missing: ${specifier}`,
			{
				cause: error,
			},
		);
	});
	if (!isWithin(canonicalPackageRoot, canonicalTarget)) {
		throw new Error(
			`[api-package-consumer] export resolved outside installed package: ${specifier}`,
		);
	}

	const actualTarget = normalizedPath(
		relative(canonicalPackageRoot, canonicalTarget),
	);
	if (actualTarget !== normalizedPath(target)) {
		throw new Error(
			`[api-package-consumer] export resolved to ${actualTarget}, want ${normalizedPath(target)}: ${specifier}`,
		);
	}

	const contents = await readFile(canonicalTarget, "utf8");
	if (canonicalTarget.endsWith(".json")) {
		let document;
		try {
			document = JSON.parse(contents);
		} catch (error) {
			throw new Error(
				`[api-package-consumer] export is not valid JSON: ${specifier}`,
				{
					cause: error,
				},
			);
		}
		await verifyJSONArtifactSemantics({
			document,
			packageRoot: canonicalPackageRoot,
			specifier,
		});
	} else if (canonicalTarget.endsWith(".yaml")) {
		if (contents.trim().length === 0) {
			throw new Error(
				`[api-package-consumer] export is empty YAML: ${specifier}`,
			);
		}
	} else {
		throw new Error(
			`[api-package-consumer] export is not a raw JSON/YAML artifact: ${specifier}`,
		);
	}

	return canonicalTarget;
}

function requireObject(value, message) {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		throw new Error(message);
	}
	return value;
}

function requireConfigurationSchema(document, specifier, expectedID) {
	const schema = requireObject(
		document,
		`[api-package-consumer] export is not a JSON Schema object: ${specifier}`,
	);
	if (
		schema.$schema !== "https://json-schema.org/draft/2020-12/schema" ||
		schema.$id !== expectedID ||
		schema.type !== "object" ||
		schema.additionalProperties !== false ||
		typeof schema.properties !== "object"
	) {
		throw new Error(
			`[api-package-consumer] export does not have the promised configuration JSON Schema semantics: ${specifier}`,
		);
	}
	return schema;
}

async function requireManifest(document, packageRoot, specifier) {
	const manifest = requireObject(
		document,
		`[api-package-consumer] export is not a contract manifest object: ${specifier}`,
	);
	if (
		manifest.formatVersion !== "1.0.0" ||
		typeof manifest.packageId !== "string" ||
		typeof manifest.packageVersion !== "string" ||
		!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(manifest.sourceCommit) ||
		Object.keys(
			requireObject(
				manifest.familyFormatVersions,
				"invalid familyFormatVersions",
			),
		).length === 0 ||
		Object.keys(requireObject(manifest.exports, "invalid exports")).length === 0
	) {
		throw new Error(
			`[api-package-consumer] export does not conform to the contract manifest shape: ${specifier}`,
		);
	}

	for (const [id, rawEntry] of Object.entries(manifest.exports)) {
		const entry = requireObject(rawEntry, `invalid manifest export ${id}`);
		if (
			typeof entry.path !== "string" ||
			!/^[0-9a-f]{64}$/.test(entry.artifactHash) ||
			typeof entry.family !== "string" ||
			typeof entry.documentation !== "object" ||
			typeof entry.lifecycle !== "object"
		) {
			throw new Error(
				`[api-package-consumer] manifest export metadata is invalid: ${id}`,
			);
		}
		const artifactPath = await realpath(
			join(packageRoot, ...entry.path.split("/")),
		);
		if (!isWithin(packageRoot, artifactPath)) {
			throw new Error(
				`[api-package-consumer] manifest artifact escapes package: ${id}`,
			);
		}
		const digest = createHash("sha256")
			.update(await readFile(artifactPath))
			.digest("hex");
		if (digest !== entry.artifactHash) {
			throw new Error(
				`[api-package-consumer] manifest artifact hash mismatch: ${id}`,
			);
		}
	}
}

async function verifyJSONArtifactSemantics({
	document,
	packageRoot,
	specifier,
}) {
	if (specifier.endsWith("/manifest")) {
		await requireManifest(document, packageRoot, specifier);
		return;
	}
	if (specifier.endsWith("/schemas/you-config")) {
		const schema = requireConfigurationSchema(
			document,
			specifier,
			"https://schemas.portpowered.com/you/config/you-config.schema.json",
		);
		if (!schema.properties.defaults || !schema.properties.workerPresets) {
			throw new Error(
				`[api-package-consumer] you-config schema is incomplete: ${specifier}`,
			);
		}
		return;
	}
	if (specifier.endsWith("/schemas/factory")) {
		const schema = requireConfigurationSchema(
			document,
			specifier,
			"https://schemas.portpowered.com/you/config/factory.schema.json",
		);
		if (
			!schema.required?.includes("name") ||
			Object.keys(schema.$defs ?? {}).length === 0
		) {
			throw new Error(
				`[api-package-consumer] Factory schema is incomplete: ${specifier}`,
			);
		}
		if (JSON.stringify(schema).includes("#/components/schemas/")) {
			throw new Error(
				`[api-package-consumer] Factory schema retains OpenAPI-only references: ${specifier}`,
			);
		}
		return;
	}
	if (specifier.endsWith("/schemas/factory-event")) {
		requireFactoryEventSchema(document, specifier);
		return;
	}
	if (specifier.endsWith("/schemas/factory-recording")) {
		const schema = requireObject(
			document,
			`[api-package-consumer] export is not a JSON Schema object: ${specifier}`,
		);
		if (
			schema.$schema !== "https://json-schema.org/draft/2020-12/schema" ||
			schema.$id !==
				"https://schemas.portpowered.com/you/factory/factory-recording.schema.json" ||
			!schema.required?.includes("schemaVersion") ||
			!schema.required?.includes("sessionId") ||
			!schema.required?.includes("events") ||
			schema.properties?.events?.items?.$ref !== "#/$defs/FactoryEvent"
		) {
			throw new Error(
				`[api-package-consumer] Factory Recording schema is incomplete: ${specifier}`,
			);
		}
		requireFactoryEventSchema(schema.$defs?.FactoryEvent, specifier);
		return;
	}
	if (specifier.endsWith("/schemas/mock-workers")) {
		const schema = requireConfigurationSchema(
			document,
			specifier,
			"https://schemas.portpowered.com/you/config/mock-workers.schema.json",
		);
		const mockWorker = schema.$defs?.mockWorker;
		if (
			!schema.required?.includes("mockWorkers") ||
			!mockWorker?.properties?.runType?.enum?.includes("script") ||
			!Array.isArray(mockWorker.allOf)
		) {
			throw new Error(
				`[api-package-consumer] mock-workers schema is incomplete: ${specifier}`,
			);
		}
	}
}

function requireFactoryEventSchema(document, specifier) {
	const schema = requireObject(
		document,
		`[api-package-consumer] export is not a Factory Event schema object: ${specifier}`,
	);
	if (
		!schema.required?.includes("schemaVersion") ||
		!schema.required?.includes("id") ||
		!schema.required?.includes("type") ||
		!schema.required?.includes("context") ||
		!schema.required?.includes("payload") ||
		schema.oneOf?.length !== 31 ||
		schema.discriminator?.propertyName !== "type" ||
		Object.keys(schema.discriminator?.mapping ?? {}).length !== 31
	) {
		throw new Error(
			`[api-package-consumer] Factory Event schema is incomplete: ${specifier}`,
		);
	}
	return schema;
}

function runNpmInstall(
	consumerDirectory,
	packageTarget,
	{ offline = false } = {},
) {
	const arguments_ = [
		"install",
		"--ignore-scripts",
		"--no-audit",
		"--no-fund",
		"--no-save",
		"--package-lock=false",
	];
	if (offline) {
		arguments_.push("--offline");
	}
	arguments_.push(packageTarget);

	return new Promise((resolvePromise, rejectPromise) => {
		const child = spawn("npm", arguments_, {
			cwd: consumerDirectory,
			shell: process.platform === "win32",
			stdio: ["ignore", "pipe", "pipe"],
		});
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
						`[api-package-consumer] npm install failed with status ${status}\n${stderr.trim()}`,
					),
				);
				return;
			}
			resolvePromise();
		});
	});
}

export async function verifyInstalledPackage({
	consumerDirectory,
	packageName,
	packedFiles,
	workspaceDirectory,
	expectedVersion,
}) {
	const consumerRoot = resolve(consumerDirectory);
	const workspaceRoot = resolve(workspaceDirectory);
	if (isWithin(workspaceRoot, consumerRoot)) {
		throw new Error(
			"[api-package-consumer] consumer must be outside the package workspace",
		);
	}

	const packageRoot = join(
		consumerRoot,
		"node_modules",
		...packageName.split("/"),
	);
	const packageStats = await lstat(packageRoot);
	if (packageStats.isSymbolicLink()) {
		throw new Error(
			"[api-package-consumer] installed package must not be a workspace link",
		);
	}

	const installedManifest = JSON.parse(
		await readFile(join(packageRoot, "package.json"), "utf8"),
	);
	if (
		typeof expectedVersion === "string" &&
		installedManifest.version !== expectedVersion
	) {
		throw new Error(
			`[api-package-consumer] installed version ${installedManifest.version}, want ${expectedVersion}`,
		);
	}
	const exportCases = concreteExportCases(installedManifest, packedFiles);
	const resolveFromConsumer = createRequire(
		join(consumerRoot, "verify.cjs"),
	).resolve;
	for (const exportCase of exportCases) {
		await verifyResolvedExport({
			packageRoot,
			resolveSpecifier: async (specifier) => resolveFromConsumer(specifier),
			...exportCase,
		});
	}

	return exportCases;
}

export async function installAndVerifyTarball(input) {
	await runNpmInstall(input.consumerDirectory, resolve(input.tarballPath), {
		offline: true,
	});
	return verifyInstalledPackage(input);
}

export async function installAndVerifyRegistryPackage(input) {
	const packageTarget = `${input.packageName}@${input.candidateVersion}`;
	await runNpmInstall(input.consumerDirectory, packageTarget);
	return verifyInstalledPackage({
		...input,
		expectedVersion: input.candidateVersion,
	});
}
