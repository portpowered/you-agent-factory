import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, relative, resolve } from "node:path";
import { pathToFileURL } from "node:url";

// Idempotent applicator for the authored localai-llamacpp independent-IMAGE
// media-boundary patch.
//
// The pinned LocalAI header `backend/cpp/llama-cpp/message_content.h` is
// copied into the prepared llama.cpp tree that the Windows CUDA recipe
// actually compiles (`<llamaSource>/tools/grpc-server/message_content.h`).
// Patching only the SDK checkout would leave the compiled header untouched, so
// the recipe applies this patch to the prepared header after `verify-source`.
//
// The authored `.patch` file is the single source of truth: the expected
// "before" and "after" regions are parsed out of its unified diff and are never
// duplicated here. Every apply is fail-closed:
//
//   * the exact expected patched region is already present once -> report
//     `already-applied` and leave the bytes untouched, so re-running against an
//     already-patched prepared tree is a no-op;
//   * the exact expected pinned region is present once -> apply and report
//     `applied`;
//   * anything else -> throw with an actionable message naming what was
//     expected and what was actually observed.

export const independentImagesPatchMarker = "LOCALAI_LLAMACPP_INDEPENDENT_IMAGE_MEDIA_BOUNDARIES";
export const independentImagesPatchTarget = "tools/grpc-server/message_content.h";
export const independentImagesPatchID = "localai-llamacpp-independent-images";
export const independentImagesTestMarker = "LOCALAI_LLAMACPP_INDEPENDENT_IMAGE_MEDIA_BOUNDARIES_TEST";

function fail(message) {
	throw new Error(message);
}

// Extracts the single-file, single-hunk unified diff regions this applicator
// guards. Anything richer than that is rejected instead of guessed, so the
// authored patch cannot silently drift away from the applied result.
function parseHunkRegions(patchText, patchPath) {
	const diffIndex = patchText.indexOf("diff --git ");
	if (diffIndex < 0) fail(`${patchPath} has no unified-diff section`);
	const lines = patchText.slice(diffIndex).split("\n");
	const minus = lines.findIndex((line) => line.startsWith("--- "));
	const plus = lines.findIndex((line) => line.startsWith("+++ "));
	if (minus < 0 || plus !== minus + 1) fail(`${patchPath} must contain exactly one file header`);
	const target = lines[plus].slice("+++ ".length).replace(/^b\//, "");
	if (target !== independentImagesPatchTarget) {
		fail(`${patchPath} must patch ${independentImagesPatchTarget}, received ${target}`);
	}
	if (lines.filter((line) => line.startsWith("diff --git ")).length !== 1) {
		fail(`${patchPath} must contain exactly one file, found a second diff --git section`);
	}
	const hunkIndexes = lines
		.map((line, index) => (line.startsWith("@@ ") ? index : -1))
		.filter((index) => index >= 0);
	if (hunkIndexes.length !== 1) {
		fail(`${patchPath} must contain exactly one hunk for ${independentImagesPatchTarget}, found ${hunkIndexes.length}`);
	}
	const hunkStart = hunkIndexes[0];
	if (hunkStart !== plus + 1) fail(`${patchPath} places a hunk header before its own +++ header`);
	const body = [];
	for (let index = hunkStart + 1; index < lines.length; index += 1) {
		const line = lines[index];
		if (line === "") {
			if (index !== lines.length - 1) fail(`${patchPath} contains an empty line inside its hunk`);
			continue;
		}
		const marker = line[0];
		if (marker !== " " && marker !== "-" && marker !== "+") {
			fail(`${patchPath} contains an unexpected hunk line: ${line}`);
		}
		body.push(line);
	}
	if (body.length === 0) fail(`${patchPath} has an empty hunk`);
	if (body[0][0] !== " " || body.at(-1)[0] !== " ") {
		fail(`${patchPath} must anchor its hunk with leading and trailing context lines`);
	}
	if (!body.some((line) => line[0] === "-" || line[0] === "+")) {
		fail(`${patchPath} must change the pinned region`);
	}
	const before = [];
	const after = [];
	for (const line of body) {
		const marker = line[0];
		const text = line.slice(1);
		if (marker === " ") {
			before.push(text);
			after.push(text);
		} else if (marker === "-") {
			before.push(text);
		} else {
			after.push(text);
		}
	}
	return { target, before: before.join("\n"), after: after.join("\n") };
}

export function parseIndependentImagesPatch(patchPath) {
	const hunk = parseHunkRegions(readFileSync(patchPath, "utf8"), patchPath);
	if (!hunk.after.includes(independentImagesPatchMarker)) {
		fail(`${patchPath} must add the ${independentImagesPatchMarker} marker so a retry can detect the same patch`);
	}
	if (hunk.before.includes(independentImagesPatchMarker)) {
		fail(`${patchPath} must not keep the ${independentImagesPatchMarker} marker in its removed lines`);
	}
	return hunk;
}

function classifyHeader(headerText, hunk, patchPath, headerPath) {
	const beforeCount = headerText.split(hunk.before).length - 1;
	const afterCount = headerText.split(hunk.after).length - 1;
	if (beforeCount === 0 && afterCount === 1) return "already-applied";
	if (beforeCount === 1 && afterCount === 0) return "applied";
	const markerCount = headerText.split(independentImagesPatchMarker).length - 1;
	if (markerCount > 0) {
		fail(
			`${patchPath} is already present but drifted from the authored patch: the marker ${independentImagesPatchMarker} appears ${markerCount} time(s) and the expected patched region appears ${afterCount} time(s) in ${headerPath}. ` +
				"Re-prepare the pinned source tree (or restore the header from the pinned LocalAI commit) and re-run; never hand-edit the patched region.",
		);
	}
	fail(
		`${patchPath} does not match ${independentImagesPatchTarget}: the expected pinned region appears ${beforeCount} time(s) and the expected patched region appears ${afterCount} time(s) in ${headerPath}. ` +
			"The prepared tree is neither the pinned shape nor this patch. Re-prepare the pinned source tree, or re-author the patch against the actual pinned header, then re-run.",
	);
	return "unreachable";
}

// Applies the authored patch to the prepared header that is actually compiled.
// Returns `{ status, patchSha256 }` where status is `applied` or
// `already-applied`.
export function applyIndependentImagesPatch({ headerPath, patchPath }) {
	const hunk = parseIndependentImagesPatch(patchPath);
	const headerText = readFileSync(headerPath, "utf8");
	const status = classifyHeader(headerText, hunk, patchPath, headerPath);
	if (status === "applied") writeFileSync(headerPath, headerText.replace(hunk.before, hunk.after), "utf8");
	return { status, patchSha256: sha256(readFileSync(patchPath)) };
}

// Stages the authored standalone CPU regression beside the prepared header so
// the recipe compiles and runs the ACTUAL patched helper, never a copy of it.
export function stageIndependentImagesTest({ testTemplatePath, destinationPath }) {
	const bytes = readFileSync(testTemplatePath);
	mkdirSync(dirname(destinationPath), { recursive: true });
	writeFileSync(destinationPath, bytes);
	return { bytes: bytes.length, testSha256: sha256(bytes) };
}

// Record the patch, CPU regression, applicator and build recipe that govern
// the native payload and its verification next to build-metadata.json.
export function independentImagesProvenance({ patchPath, testTemplatePath, repositoryRoot = process.cwd() }) {
	const hunk = parseIndependentImagesPatch(patchPath);
	const testBytes = readFileSync(testTemplatePath);
	if (!testBytes.includes(independentImagesPatchMarker) || !testBytes.includes(independentImagesTestMarker)) {
		fail(`${testTemplatePath} must exercise the patched ${independentImagesPatchMarker} helper`);
	}
	return {
		formatVersion: 1,
		id: independentImagesPatchID,
		target: hunk.target,
		marker: independentImagesPatchMarker,
		inputs: [
			patchPath,
			testTemplatePath,
			resolve(repositoryRoot, "scripts/localai-llamacpp-independent-images-patch.mjs"),
			resolve(repositoryRoot, "scripts/build-localai-backend-cuda.ps1"),
		].map((path) => {
			const bytes = readFileSync(path);
			return { path: repositoryRelative(repositoryRoot, path), sha256: sha256(bytes), bytes: bytes.length };
		}),
	};
}

function repositoryRelative(repositoryRoot, path) {
	const target = resolve(path);
	const rel = relative(resolve(repositoryRoot), target);
	return rel && !rel.startsWith("..") ? rel.replaceAll("\\", "/") : target.replaceAll("\\", "/");
}

function sha256(bytes) {
	return createHash("sha256").update(bytes).digest("hex");
}

function option(args, name) {
	const index = args.indexOf(name);
	if (index < 0 || !args[index + 1]) fail(`${name} is required`);
	return args[index + 1];
}

function main(args) {
	const command = args[0];
	if (command === "apply") {
		const patchPath = option(args, "--patch");
		const result = applyIndependentImagesPatch({ headerPath: option(args, "--header"), patchPath });
		const staged = stageIndependentImagesTest({
			testTemplatePath: option(args, "--test-template"),
			destinationPath: option(args, "--test-destination"),
		});
		process.stdout.write(
			`LOCALAI_INDEPENDENT_IMAGES_PATCH_OK status=${result.status} patch_sha256=${result.patchSha256} test_sha256=${staged.testSha256}\n`,
		);
		return;
	}
	if (command === "provenance") {
		const provenance = independentImagesProvenance({
			patchPath: option(args, "--patch"),
			testTemplatePath: option(args, "--test-template"),
			repositoryRoot: option(args, "--repository-root"),
		});
		const outputPath = option(args, "--output");
		mkdirSync(dirname(outputPath), { recursive: true });
		writeFileSync(outputPath, `${JSON.stringify(provenance, null, 2)}\n`, "utf8");
		process.stdout.write(
			`LOCALAI_INDEPENDENT_IMAGES_PROVENANCE_OK id=${provenance.id} inputs=${provenance.inputs.length}\n`,
		);
		return;
	}
	fail("usage: localai-llamacpp-independent-images-patch.mjs apply|provenance [options]");
}

if (import.meta.url === pathToFileURL(resolve(process.argv[1] ?? "")).href) {
	try {
		main(process.argv.slice(2));
	} catch (error) {
		console.error(error instanceof Error ? error.message : error);
		process.exit(1);
	}
}