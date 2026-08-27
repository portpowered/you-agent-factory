import { spawn } from "node:child_process";
import { closeSync, mkdirSync, openSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const [ordinal, outputDirectory, command, ...args] = process.argv.slice(2);

if (!ordinal || !outputDirectory || !command) {
	console.error(
		"usage: node run-unit-latency-sample.mjs <ordinal> <output-directory> <command> [args...],",
	);
	process.exitCode = 2;
} else {
	const stdoutPath = join(outputDirectory, `run-${ordinal}.stdout.log`);
	const stderrPath = join(outputDirectory, `run-${ordinal}.stderr.log`);
	const statusPath = join(outputDirectory, `run-${ordinal}.status.txt`);
	mkdirSync(outputDirectory, { recursive: true });

	const stdoutFile = openSync(stdoutPath, "w");
	const stderrFile = openSync(stderrPath, "w");
	let status = 1;
	let launchError;
	try {
		const child = spawn(command, args, {
			stdio: ["ignore", stdoutFile, stderrFile],
		});
		status = await new Promise((resolve) => {
			let settled = false;
			const finish = (value) => {
				if (settled) {
					return;
				}
				settled = true;
				resolve(value);
			};
			child.once("error", (error) => {
				launchError = error;
				finish(127);
			});
			child.once("close", (code) => finish(code ?? 1));
		});
	} finally {
		closeSync(stdoutFile);
		closeSync(stderrFile);
	}

	writeFileSync(statusPath, `exit_status=${status}\n`, "utf8");
	if (launchError) {
		console.error(`failed to launch ${command}: ${launchError.message}`);
	}
	process.exitCode = status === 0 ? 0 : 1;
}
