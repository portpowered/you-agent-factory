import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { delimiter, join } from "node:path";

export function testBash({ requireSetsid = false } = {}) {
	if (process.env.BASH_BIN) return process.env.BASH_BIN;
	if (process.platform === "win32" && process.env.MSYSTEM) {
		const gitBash = process.env.PATH.split(delimiter)
			.map((directory) => join(directory, "bash.exe"))
			.find((candidate) => existsSync(candidate) && hasCommand(candidate, "cygpath"));
		if (gitBash && (!requireSetsid || hasCommand(gitBash, "setsid"))) return gitBash;
	}
	if (requireSetsid && process.platform === "win32") {
		const wslBash = join(process.env.SystemRoot || "C:\\Windows", "System32", "bash.exe");
		if (existsSync(wslBash) && hasCommand(wslBash, "setsid")) return wslBash;
	}
	return "bash";
}

function hasCommand(bash, command) {
	const probe = spawnSync(bash, ["-c", `command -v ${command} >/dev/null`], {
		windowsHide: true,
	});
	return !probe.error && probe.status === 0;
}

export function pathForTestBash(path, bash = testBash()) {
	if (process.platform !== "win32" || !path) return path;
	const quoted = "'" + path.replaceAll("'", "'\"'\"'") + "'";
	const result = spawnSync(bash, ["-c", `if command -v cygpath >/dev/null; then cygpath -u ${quoted}; else wslpath -u ${quoted}; fi`], {
		encoding: "utf8",
		windowsHide: true,
	});
	if (result.error || result.status !== 0) {
		throw new Error(`cannot map path for ${bash}: ${result.error?.message || result.stderr}`);
	}
	return result.stdout.trim();
}
