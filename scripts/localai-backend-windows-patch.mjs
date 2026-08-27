import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

export function patchWindowsGoLoader({ mainPath, loaderPath, libraryName, backendID }) {
	if (!/^[A-Za-z0-9._-]+\.dll$/.test(libraryName)) {
		throw new Error(`unsafe Windows backend library name: ${libraryName}`);
	}

	let source = readFileSync(mainPath, "utf8");
	const loaderNeedle = "purego.Dlopen(libName, purego.RTLD_NOW|purego.RTLD_GLOBAL)";
	const loaderOccurrences = source.split(loaderNeedle).length - 1;
	if (loaderOccurrences !== 1) {
		throw new Error(`expected one purego dynamic-loader call in ${mainPath}, found ${loaderOccurrences}`);
	}
	source = source.replace(loaderNeedle, "loadBackendLibrary(libName)");

	const fallbackPattern = /(\t\t\} else \{\r?\n\t\t\tlibName = )"([^"]+\.so)"(\r?\n\t\t\})/g;
	const fallbackMatches = [...source.matchAll(fallbackPattern)];
	if (fallbackMatches.length !== 1 || !fallbackMatches[0][2].startsWith("./")) {
		throw new Error(`expected one relative Unix fallback library branch in ${mainPath}`);
	}
	const fallback = fallbackMatches[0];
	const newline = fallback[0].includes("\r\n") ? "\r\n" : "\n";
	const windowsBranch = [
		'\t\t} else if (runtime.GOOS == "windows") {',
		`\t\t\tlibName = "./${libraryName}"`,
		"\t\t} else {",
		`\t\t\tlibName = "${fallback[2]}"`,
		"\t\t}",
	].join(newline);
	source = source.replace(fallback[0], windowsBranch);

	let callbackShim = "";
	if (backendID === "localai-whisper") {
		const callbackNeedle = "purego.NewCallback(onNewSegment)";
		const callbackOccurrences = source.split(callbackNeedle).length - 1;
		if (callbackOccurrences !== 1) {
			throw new Error(`expected one Whisper callback registration in ${mainPath}, found ${callbackOccurrences}`);
		}
		source = source.replace(callbackNeedle, "purego.NewCallback(localAINewSegmentCallback)");
		callbackShim = [
			"",
			"// syscall.NewCallback requires a uintptr-sized result on Windows. The",
			"// C callback is void; its ABI-compatible return value is ignored.",
			"func localAINewSegmentCallback(idxFirst int32, nNew int32, userData uintptr) uintptr {",
			"\tonNewSegment(idxFirst, nNew, userData)",
			"\treturn 0",
			"}",
		].join("\n");
	}

	writeFileSync(
		loaderPath,
		`//go:build windows

package main

import "golang.org/x/sys/windows"

func loadBackendLibrary(name string) (uintptr, error) {
	handle, err := windows.LoadLibrary(name)
	return uintptr(handle), err
}
${callbackShim}
`,
	);
	writeFileSync(mainPath, source);
}

function runCLI() {
	const [mainPath, loaderPath, libraryName, backendID] = process.argv.slice(2);
	if (!mainPath || !loaderPath || !libraryName || !backendID) {
		throw new Error("usage: localai-backend-windows-patch.mjs <main> <loader> <library.dll> <backend>");
	}
	patchWindowsGoLoader({ mainPath, loaderPath, libraryName, backendID });
}

if (import.meta.url === pathToFileURL(resolve(process.argv[1] ?? "")).href) {
	try {
		runCLI();
	} catch (error) {
		console.error(error instanceof Error ? error.message : error);
		process.exit(1);
	}
}
