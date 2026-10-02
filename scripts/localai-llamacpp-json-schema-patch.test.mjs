import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { applySchemaPatch, extractSchemaHelper, schemaProvenance } from './localai-llamacpp-json-schema-patch.mjs';

const logicalPatch = () => readFileSync(new URL('./localai-llamacpp-json-schema.patch', import.meta.url), 'utf8')
	.replace(/\r\n/g, '\n');

function preimage(eol) {
	const old = [];
	let hunk = false;
	for (const line of logicalPatch().split('\n')) {
		if (line.startsWith('@@ ')) { hunk = true; old.push('\n\n\n'); continue; }
		if (hunk && (line.startsWith(' ') || (line.startsWith('-') && !line.startsWith('---')))) old.push(line.slice(1) + '\n');
	}
	return old.join('').replace(/\n/g, eol);
}

function fixture(t, { serverEol, patchEol }) {
	const root = mkdtempSync(join(tmpdir(), 'you-schema-patch-'));
	t.after(() => rmSync(root, { recursive: true, force: true }));
	const server = join(root, 'tools/grpc-server/grpc-server.cpp');
	mkdirSync(join(root, 'tools/grpc-server'), { recursive: true });
	writeFileSync(server, preimage(serverEol));
	const patchFile = join(root, 'schema.patch');
	const authored = Buffer.from(logicalPatch().replace(/\n/g, patchEol), 'utf8');
	writeFileSync(patchFile, authored);
	const testTemplate = join(root, 'fixture.cpp.in');
	writeFileSync(testTemplate, '// fixture\n');
	return { paths: { server, patch: patchFile, testTemplate, testDestination: join(root, 'cpu/fixture.cpp'),
		helperDestination: join(root, 'cpu/request_json_schema_test_helper.h') }, authored };
}

function assertUniformEol(text, eol) {
	const crlf = (text.match(/\r\n/g) || []).length;
	const bare = (text.match(/(?<!\r)\n/g) || []).length;
	assert.equal(crlf > 0, eol === '\r\n', `line endings were rewritten: ${JSON.stringify(text)}`);
	if (eol === '\r\n') assert.equal(bare, 0, `bare LF survived a CRLF apply: ${JSON.stringify(text)}`);
	else assert.equal(crlf, 0, `CR survived an LF apply: ${JSON.stringify(text)}`);
}

// The authored schema patch ships with CRLF bytes while the pinned prepared llama.cpp
// server is LF; the applicator must reconcile that without touching either byte stream.
for (const [serverEol, patchEol] of [['\n', '\r\n'], ['\r\n', '\n']]) {
	const label = `prepared ${serverEol === '\n' ? 'LF' : 'CRLF'} server with ${patchEol === '\n' ? 'LF' : 'CRLF'} authored patch`;

	test(`schema apply is repeatable and generated helper binds actual compiled bytes: ${label}`, t => {
		const { paths, authored } = fixture(t, { serverEol, patchEol });
		assert.equal(applySchemaPatch(paths).status, 'applied');
		const compiled = readFileSync(paths.server, 'utf8');
		assertUniformEol(compiled, serverEol);
		assert.equal(applySchemaPatch(paths).status, 'already-applied');
		assert.equal(readFileSync(paths.server, 'utf8'), compiled);
		assert.deepEqual(readFileSync(paths.patch), authored);
		assert.ok(readFileSync(paths.helperDestination, 'utf8').endsWith(extractSchemaHelper(compiled)));
		assert.ok(readFileSync(paths.testDestination, 'utf8').startsWith('#include "request_json_schema_test_helper.h"'));
		assert.match(schemaProvenance(paths).files.server.sha256, /^[0-9a-f]{64}$/);
	});

	test(`schema apply refuses incompatible or partial source without mutation: ${label}`, t => {
		const { paths, authored } = fixture(t, { serverEol, patchEol });
		const incompatible = ['unknown prepared source',
			'// LOCALAI_LLAMACPP_REQUEST_JSON_SCHEMA_HEALTH\n'].map(source => source.replace(/\n/g, serverEol));
		for (const source of incompatible) {
			writeFileSync(paths.server, source);
			assert.throws(() => applySchemaPatch(paths));
			assert.equal(readFileSync(paths.server, 'utf8'), source);
			assert.deepEqual(readFileSync(paths.patch), authored);
		}
	});
}

test('schema apply refuses mixed prepared line endings without mutation', t => {
	const { paths, authored } = fixture(t, { serverEol: '\n', patchEol: '\r\n' });
	const mixed = '\r\n' + readFileSync(paths.server, 'utf8');
	writeFileSync(paths.server, mixed);
	assert.throws(() => applySchemaPatch(paths), /prepared source mixes line endings/);
	assert.equal(readFileSync(paths.server, 'utf8'), mixed);
	assert.deepEqual(readFileSync(paths.patch), authored);
});
