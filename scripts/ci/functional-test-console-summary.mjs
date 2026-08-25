import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

function readJson(path) {
	try {
		return JSON.parse(readFileSync(path, "utf8"));
	} catch {
		return null;
	}
}

function displayPath(importPath, marker) {
	const index = importPath.indexOf(marker);
	return index >= 0 ? importPath.slice(index + 1) : importPath;
}

export function renderFunctionalTestConsoleSummary(coverage, timing) {
	const lines = [];
	const packages = Array.isArray(coverage?.packages)
		? coverage.packages
				.filter(
					(entry) =>
						typeof entry?.package === "string" &&
						entry.package.includes("/pkg/") &&
						typeof entry.coveragePercent === "number" &&
						Number.isFinite(entry.coveragePercent),
				)
				.sort((left, right) => left.package.localeCompare(right.package))
		: [];
	if (packages.length > 0) {
		lines.push("Functional coverage for pkg/:");
		for (const entry of packages) {
			lines.push(
				`${displayPath(entry.package, "/pkg/")} ${entry.coveragePercent.toFixed(1)}%`,
			);
		}
	}

	const tests = Array.isArray(timing?.tests)
		? timing.tests
				.filter(
					(entry) =>
						typeof entry?.package === "string" &&
						entry.package.includes("/tests/functional/") &&
						typeof entry.test === "string" &&
						typeof entry.seconds === "number" &&
						Number.isFinite(entry.seconds),
				)
				.sort((left, right) => {
					const packageOrder = left.package.localeCompare(right.package);
					return packageOrder === 0
						? left.test.localeCompare(right.test)
						: packageOrder;
				})
		: [];
	if (tests.length > 0) {
		if (lines.length > 0) {
			lines.push("");
		}
		lines.push("Functional test latencies:");
		for (const entry of tests) {
			lines.push(
				`${displayPath(entry.package, "/tests/functional/")} ${entry.test} ${entry.seconds.toFixed(3)}s`,
			);
		}
	}

	return lines.length > 0 ? `${lines.join("\n")}\n` : "";
}

function optionValue(args, name) {
	const index = args.indexOf(name);
	if (index === -1 || !args[index + 1]) {
		throw new Error(`${name} is required`);
	}
	return args[index + 1];
}

function runCli(args) {
	const coveragePath = optionValue(args, "--coverage");
	const timingPath = optionValue(args, "--timing");
	process.stdout.write(
		renderFunctionalTestConsoleSummary(
			readJson(coveragePath),
			readJson(timingPath),
		),
	);
}

const invokedPath = process.argv[1]
	? pathToFileURL(resolve(process.argv[1])).href
	: "";
if (invokedPath === import.meta.url) {
	try {
		runCli(process.argv.slice(2));
	} catch (error) {
		console.error(
			`Functional test console summary unavailable: ${error.message}`,
		);
		process.exitCode = 1;
	}
}
