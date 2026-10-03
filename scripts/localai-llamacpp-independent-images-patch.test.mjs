import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
 applyIndependentImagesPatch,
 independentImagesPatchMarker,
 independentImagesPatchTarget,
 independentImagesProvenance,
 parseIndependentImagesPatch,
 stageIndependentImagesTest,
} from "./localai-llamacpp-independent-images-patch.mjs";

const independentImagesPatchPath = "scripts/localai-llamacpp-independent-images.patch";
const independentImagesTestPath = "scripts/localai-llamacpp-independent-images_test.cpp.in";

// The pinned LocalAI grpc-server header, reduced to the region the authored patch
// touches. The replaced region itself is taken from the authored patch so the
// fixture can never drift from what the patch expects; test
// "the authored patch removes the pinned adjacency defect" below pins the pinned
// shape independently.
async function pinnedHeaderFixture(t) {
	const root = await mkdtemp(join(tmpdir(), "localai-independent-images-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	const hunk = parseIndependentImagesPatch(independentImagesPatchPath);
	const headerPath = join(root, "message_content.h");
	await writeFile(
		headerPath,
		[
			"#pragma once",
			"",
			"#include <string>",
			"#include <vector>",
			"",
			"#include <nlohmann/json.hpp>",
			"",
			"namespace llama_grpc {",
			"",
			hunk.before,
			"",
			"}  // namespace llama_grpc",
			"",
		].join("\n"),
	);
	return { root, headerPath, hunk };
}

test("the authored patch targets the prepared header that the CUDA build actually compiles", async () => {
	const hunk = parseIndependentImagesPatch(independentImagesPatchPath);
	assert.equal(hunk.target, independentImagesPatchTarget);
	assert.equal(hunk.target, "tools/grpc-server/message_content.h");
	// The patched header lives in the prepared tree, which is copied from
	// LOCALAI_ROOT/backend/cpp/llama-cpp and is what CMake compiles.
	assert.equal(
		independentImagesPatchTarget,
		join("tools", "grpc-server", "message_content.h").replaceAll("\\", "/"),
	);
	assert.match(hunk.after, new RegExp(`\\n// ${independentImagesPatchMarker}:`));
	assert.match(hunk.after, /append_media_boundary\("Image", index \+ 1\)/);
	assert.match(hunk.after, /append_media_boundary\("Video", index \+ 1\)/);
	// A retry must be able to recognise the same patch.
	assert.ok(hunk.after.includes(independentImagesPatchMarker));
	assert.ok(!hunk.before.includes(independentImagesPatchMarker));
});

test("the authored patch removes the pinned adjacency defect and adds no model-specific branch", async () => {
	const hunk = parseIndependentImagesPatch(independentImagesPatchPath);
	// The removed region is the pinned, defect-free-of-boundaries shape: three
	// contiguous push_back loops with no separator between independent images.
	assert.match(hunk.before, /for \(const auto& img : images\) \{/);
	assert.match(hunk.before, /content_array\.push_back\(image_chunk\);/);
	assert.doesNotMatch(hunk.before, /append_media_boundary/);
	assert.match(hunk.before, /for \(const auto& aud : audios\) \{/);
	assert.match(hunk.before, /for \(const auto& vid : videos\) \{/);
	// Ordinals only; no model, projector, template, or user-facing flag gate in
	// the executable code. Comments may cite a template by name as rationale.
	const patchedCode = hunk.after
		.split("\n")
		.filter((line) => !line.trimStart().startsWith("//"))
		.join("\n");
	assert.doesNotMatch(patchedCode, /qwen|gemma|projector|env|getenv|argv|flag|model/i);
	assert.match(patchedCode, /append_media_boundary\("Image", index \+ 1\)/);
	assert.match(patchedCode, /append_media_boundary\("Video", index \+ 1\)/);
	// Ordinals are the only content introduced; no invented per-model labels.
	assert.deepEqual(
		[...patchedCode.matchAll(/append_media_boundary\("([A-Za-z]+)", index \+ 1\)/g)].map((match) => match[1]),
		["Image", "Video"],
	);
	// The user prompt is not touched: the patch only rewrites append_media_parts,
	// and the surrounding reconstruction keeps normalize_message_content intact.
	const header = await readFile(independentImagesPatchPath, "utf8");
	assert.doesNotMatch(header, /normalize_message_content/);
	assert.doesNotMatch(header, /build_reconstructed_message/);
	// Audio routing is preserved verbatim.
	assert.match(hunk.after, /input_audio\["format"\] = "wav"; \/\/ default; could be made configurable/);
});

test("the independent-IMAGE patch applies to the prepared header, retries as a no-op, and fails closed on an unknown shape", async (t) => {
	const { root, headerPath, hunk } = await pinnedHeaderFixture(t);
	const pinned = await readFile(headerPath, "utf8");

	const first = applyIndependentImagesPatch({ headerPath, patchPath: independentImagesPatchPath });
	assert.equal(first.status, "applied");
	assert.match(first.patchSha256, /^[0-9a-f]{64}$/);
	const patched = await readFile(headerPath, "utf8");
	assert.notEqual(patched, pinned);
	assert.ok(patched.includes(hunk.after));
	assert.ok(!patched.includes(hunk.before));
	// Exactly one boundary helper and one marker: no accidental double application.
	assert.equal(patched.split(independentImagesPatchMarker).length - 1, 1);
	assert.equal(patched.split("auto append_media_boundary = ").length - 1, 1);
	// The rest of the header is untouched.
	assert.equal(patched.replace(hunk.after, hunk.before), pinned);

	// Retry over the already-patched tree detects the same patch and leaves the
	// bytes exactly as they were.
	const second = applyIndependentImagesPatch({ headerPath, patchPath: independentImagesPatchPath });
	assert.equal(second.status, "already-applied");
	assert.equal(second.patchSha256, first.patchSha256);
	assert.equal(await readFile(headerPath, "utf8"), patched);

	// Unknown shape: neither the pinned region nor the patched region is present.
	const unknownPath = join(root, "unknown.h");
	await writeFile(unknownPath, "#pragma once\n// an unrelated header\n");
	assert.throws(
		() => applyIndependentImagesPatch({ headerPath: unknownPath, patchPath: independentImagesPatchPath }),
		(error) => {
			assert.match(error.message, /neither the pinned shape nor this patch/);
			assert.match(error.message, /Re-prepare the pinned source tree/);
			assert.match(error.message, /expected pinned region appears 0 time\(s\)/);
			return true;
		},
	);
	assert.equal(await readFile(unknownPath, "utf8"), "#pragma once\n// an unrelated header\n");

	// Drifted patch: the marker survives but the body changed, so the retry must
	// refuse instead of reporting a false success.
	const driftedPath = join(root, "drifted.h");
	await writeFile(driftedPath, patched.replace('boundary["text"] = std::string(kind)', 'boundary["text"] = std::string(kind) /* drift */'));
	assert.throws(
		() => applyIndependentImagesPatch({ headerPath: driftedPath, patchPath: independentImagesPatchPath }),
		(error) => {
			assert.match(error.message, /already present but drifted from the authored patch/);
			assert.match(error.message, /never hand-edit the patched region/);
			return true;
		},
	);

	// Duplicated pinned region is also refused rather than partially applied.
	const doubledPath = join(root, "doubled.h");
	await writeFile(doubledPath, `${pinned}\n${pinned}`);
	assert.throws(
		() => applyIndependentImagesPatch({ headerPath: doubledPath, patchPath: independentImagesPatchPath }),
		/expected pinned region appears 2 time\(s\)/,
	);
});

test("the independent-IMAGE regression is staged beside the prepared header as authored bytes", async (t) => {
	const { root } = await pinnedHeaderFixture(t);
	const destination = join(root, "nested", "grpc-server", "message_content_independent_images_test.cpp");
	const staged = stageIndependentImagesTest({ testTemplatePath: independentImagesTestPath, destinationPath: destination });
	const authored = await readFile(independentImagesTestPath);
	assert.equal(staged.bytes, authored.byteLength);
	assert.equal(staged.testSha256, createHash("sha256").update(authored).digest("hex"));
	assert.deepEqual(await readFile(destination), authored);
	// Never a bare .cpp in the repository scripts package; the convention is .cpp.in.
	assert.ok(independentImagesTestPath.endsWith(".cpp.in"));
});

test("the independent-IMAGE authored inputs are recorded as repeatable build inputs", async () => {
	const provenance = independentImagesProvenance({
		patchPath: independentImagesPatchPath,
		testTemplatePath: independentImagesTestPath,
		repositoryRoot: process.cwd(),
	});
	assert.equal(provenance.id, "localai-llamacpp-independent-images");
	assert.equal(provenance.target, independentImagesPatchTarget);
	assert.equal(provenance.marker, independentImagesPatchMarker);
	assert.deepEqual(
		provenance.inputs.map((input) => input.path),
		[independentImagesPatchPath, independentImagesTestPath,
			"scripts/localai-llamacpp-independent-images-patch.mjs", "scripts/build-localai-backend-cuda.ps1"],
	);
	for (const input of provenance.inputs) {
		const bytes = await readFile(input.path);
		assert.equal(input.bytes, bytes.byteLength);
		assert.equal(input.sha256, createHash("sha256").update(bytes).digest("hex"));
		assert.match(input.sha256, /^[0-9a-f]{64}$/);
	}
});
