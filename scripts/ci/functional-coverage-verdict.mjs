import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const inventoryPrefix = "Functional suite inventory:";
const ordinaryGocoverageExitCode = 1;
const advisoryPolicyBanner = "!!! COVERAGE FLOOR POLICY: advisory !!!";
const failedCoverageTestDiagnostic = "package floors were NOT checked because the coverage test run failed";

export function parseRecordedExitCode(value, source = "recorded exit code") {
	const normalized = String(value).trim();
	if (!/^\d+$/.test(normalized)) {
		throw new Error(`${source} must contain one non-negative integer (got ${JSON.stringify(normalized)})`);
	}
	const exitCode = Number(normalized);
	if (!Number.isSafeInteger(exitCode) || exitCode > 255) {
		throw new Error(`${source} is outside the supported process exit-code range (got ${normalized})`);
	}
	return exitCode;
}

export function readRecordedExitCode(path) {
	return parseRecordedExitCode(readFileSync(path, "utf8"), path);
}

function normalizedLines(log) {
	return String(log).replace(/\r\n?/g, "\n").split("\n");
}

function isCompactVerdictLine(line) {
	return (
		line === advisoryPolicyBanner ||
		line.startsWith("Package floors and missing-manifest findings are report-only") ||
		line.startsWith("Set -package-floor-policy=blocking to restore blocking enforcement.") ||
		line.startsWith(inventoryPrefix) ||
		line.startsWith("total: (statements)") ||
		line.startsWith("Functional package coverage verdict:") ||
		line.startsWith("  floor violation:") ||
		line.startsWith("  floor hold:") ||
		line === "  floor violations: none" ||
		line.startsWith("  package=") ||
		line.startsWith("  tally:") ||
		line.startsWith("package coverage regression:") ||
		line.startsWith("package coverage hold:") ||
		line.startsWith("coverage manifest missing entry:") ||
		line.startsWith("coverage not evaluated:") ||
		/^(?:go|Go) coverage (?:found |.* below minimum |.* meets minimum )/.test(line)
	);
}

function hasCoverageGateFailure(lines) {
	return lines.some(
		(line) =>
			line.startsWith("package coverage regression:") ||
			line.startsWith("go coverage found ") ||
			/^go coverage .* below minimum /.test(line) ||
			/\bgate-failures=[1-9]\d*/.test(line),
	);
}

function hasOrdinaryTestFailure(lines) {
	return lines.some(
		(line) =>
			line.startsWith("coverage not evaluated: ") &&
			line.includes(failedCoverageTestDiagnostic),
	);
}

function hasGreenGate(lines) {
	return lines.some((line) => /^Go coverage .* meets minimum /.test(line));
}

function isAdvisoryPolicyLine(line) {
	return (
		line === advisoryPolicyBanner ||
		line.startsWith("Package floors and missing-manifest findings are report-only") ||
		line.startsWith("Set -package-floor-policy=blocking to restore blocking enforcement.")
	);
}

function hasAdvisoryFinding(lines) {
	return lines.some(
		(line) =>
			line.startsWith("package coverage regression:") ||
			line.startsWith("coverage manifest missing entry:") ||
			line.startsWith("coverage not evaluated: package=") ||
			line.startsWith("package coverage warning:"),
	);
}

export function extractFunctionalCoverageVerdict(log) {
	const lines = normalizedLines(log);
	// Use the terminal inventory when a test itself happens to print the same
	// prefix earlier in the full stream.
	const inventoryIndex = lines.findLastIndex((line) => line.startsWith(inventoryPrefix));
	if (inventoryIndex < 0) {
		return {
			foundInventory: false,
			text: "",
			lines: [],
			hasCoverageGateFailure: false,
			hasOrdinaryTestFailure: false,
			hasGreenGate: false,
			hasAdvisoryPolicy: false,
			hasAdvisoryFindings: false,
		};
	}

	const tail = lines.slice(inventoryIndex);
	const advisoryLines = lines.filter(isAdvisoryPolicyLine);
	const verdictLines = [
		...advisoryLines,
		...tail.filter((line) => isCompactVerdictLine(line) && !isAdvisoryPolicyLine(line)),
	];
	const text = verdictLines.length > 0 ? `${verdictLines.join("\n")}\n` : "";
	return {
		foundInventory: true,
		text,
		lines: verdictLines,
		hasCoverageGateFailure: hasCoverageGateFailure(tail),
		hasOrdinaryTestFailure: hasOrdinaryTestFailure(tail),
		hasGreenGate: hasGreenGate(tail),
		hasAdvisoryPolicy: advisoryLines.length > 0,
		hasAdvisoryFindings: hasAdvisoryFinding(lines),
	};
}

export function classifyFunctionalCoverageRun({ commandExitCode = 0, gocoverageExitCode, log }) {
	const extraction = extractFunctionalCoverageVerdict(log);
	if (commandExitCode === 124 || commandExitCode === 125) {
		return {
			outcome: "timeout",
			shouldDeferFailure: false,
			exitCode: commandExitCode,
			extraction,
			reason: `functional runner exited with timeout status ${commandExitCode}`,
		};
	}
	if (commandExitCode !== 0) {
		return {
			outcome: "infrastructure-failure",
			shouldDeferFailure: false,
			exitCode: commandExitCode,
			extraction,
			reason: `functional runner exited with infrastructure status ${commandExitCode}`,
		};
	}
	if (!Number.isInteger(gocoverageExitCode)) {
		return {
			outcome: "infrastructure-failure",
			shouldDeferFailure: false,
			exitCode: 1,
			extraction,
			reason: "gocoveragecheck did not record an exit code",
		};
	}
	if (gocoverageExitCode === 0 && extraction.foundInventory && extraction.hasGreenGate) {
		const advisoryOnly = extraction.hasAdvisoryPolicy && extraction.hasAdvisoryFindings;
		return {
			outcome: advisoryOnly ? "advisory" : "green",
			shouldDeferFailure: false,
			exitCode: 0,
			extraction,
			reason: advisoryOnly
				? "functional coverage passed with report-only package-floor findings"
				: "functional coverage and test gates passed",
		};
	}
	if (
		gocoverageExitCode === ordinaryGocoverageExitCode &&
		extraction.foundInventory &&
		extraction.hasOrdinaryTestFailure
	) {
		return {
			outcome: "test-failure",
			shouldDeferFailure: true,
			exitCode: gocoverageExitCode,
			extraction,
			reason: "gocoveragecheck recorded an ordinary test failure",
		};
	}
	if (
		gocoverageExitCode === ordinaryGocoverageExitCode &&
		extraction.foundInventory &&
		extraction.hasCoverageGateFailure
	) {
		return {
			outcome: "coverage-gate-failure",
			shouldDeferFailure: true,
			exitCode: gocoverageExitCode,
			extraction,
			reason: "gocoveragecheck recorded a coverage-gate failure",
		};
	}
	if (extraction.foundInventory) {
		return {
			outcome: "incomplete",
			shouldDeferFailure: false,
			exitCode: gocoverageExitCode || 1,
			extraction,
			reason: `gocoveragecheck exit ${gocoverageExitCode} had no recognized complete verdict`,
		};
	}
	return {
		outcome: "infrastructure-failure",
		shouldDeferFailure: false,
		exitCode: gocoverageExitCode || 1,
		extraction,
		reason: `gocoveragecheck exit ${gocoverageExitCode} had no recognized complete verdict`,
	};
}

function parseArguments(argv) {
	const values = {};
	for (let index = 0; index < argv.length; index += 1) {
		const argument = argv[index];
		if (!argument.startsWith("--") || index + 1 >= argv.length) {
			throw new Error(`expected a value after ${argument}`);
		}
		values[argument.slice(2)] = argv[index + 1];
		index += 1;
	}
	for (const name of ["log", "exit-code-file", "output"]) {
		if (!values[name]) {
			throw new Error(`--${name} is required`);
		}
	}
	return values;
}

export function writeFunctionalCoverageVerdict({ logPath, exitCodePath, outputPath }) {
	const log = readFileSync(logPath, "utf8");
	const gocoverageExitCode = readRecordedExitCode(exitCodePath);
	const classification = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode,
		log,
	});
	if (["infrastructure-failure", "incomplete"].includes(classification.outcome)) {
		throw new Error(classification.reason);
	}
	if (!classification.extraction.text) {
		throw new Error("functional coverage verdict extract is empty");
	}
	mkdirSync(dirname(resolve(outputPath)), { recursive: true });
	const output = `Functional coverage outcome: ${classification.outcome}\n${classification.extraction.text}`;
	writeFileSync(outputPath, output, "utf8");
	return classification;
}

function runCli(argv) {
	const args = parseArguments(argv);
	const classification = writeFunctionalCoverageVerdict({
		logPath: args.log,
		exitCodePath: args["exit-code-file"],
		outputPath: args.output,
	});
	console.log(
		`Functional coverage capture: outcome=${classification.outcome} ` +
		`gocoveragecheck-exit-code=${classification.exitCode} ` +
		`verdict-bytes=${Buffer.byteLength(classification.extraction.text, "utf8")}`,
	);
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
	try {
		runCli(process.argv.slice(2));
	} catch (error) {
		console.error(`Functional coverage verdict unavailable: ${error.message}`);
		process.exitCode = 1;
	}
}
