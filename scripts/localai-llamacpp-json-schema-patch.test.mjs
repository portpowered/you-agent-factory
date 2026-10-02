import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { applySchemaPatch, extractSchemaHelper, schemaProvenance } from './localai-llamacpp-json-schema-patch.mjs';

function fixture(t) {
	const root = mkdtempSync(join(tmpdir(), 'you-schema-patch-'));
	t.after(() => rmSync(root, { recursive: true, force: true }));
	const patch = new URL('./localai-llamacpp-json-schema.patch', import.meta.url);
	const text = readFileSync(patch, 'utf8').replace(/\r\n/g, '\n');
	const old = [];
	let hunk = false;
	for (const line of text.split('\n')) {
		if (line.startsWith('@@ ')) { hunk = true; old.push('\n\n\n'); continue; }
		if (hunk && (line.startsWith(' ') || (line.startsWith('-') && !line.startsWith('---')))) old.push(line.slice(1) + '\n');
	}
	const server = join(root, 'tools/grpc-server/grpc-server.cpp');
	mkdirSync(join(root, 'tools/grpc-server'), { recursive: true });
	writeFileSync(server, old.join(''));
	const patchFile = join(root, 'schema.patch');
	writeFileSync(patchFile, text);
	const testTemplate = join(root, 'fixture.cpp.in');
	writeFileSync(testTemplate, '// fixture\n');
	return { server, patch: patchFile, testTemplate, testDestination: join(root, 'cpu/fixture.cpp'),
		helperDestination: join(root, 'cpu/request_json_schema_test_helper.h') };
}

test('schema apply is repeatable and generated helper binds actual compiled bytes', t => {
	const paths = fixture(t);
	assert.equal(applySchemaPatch(paths).status, 'applied');
	const compiled = readFileSync(paths.server, 'utf8');
	assert.equal(applySchemaPatch(paths).status, 'already-applied');
	assert.equal(readFileSync(paths.server, 'utf8'), compiled);
	assert.ok(readFileSync(paths.helperDestination, 'utf8').endsWith(extractSchemaHelper(compiled)));
	assert.ok(readFileSync(paths.testDestination, 'utf8').startsWith('#include "request_json_schema_test_helper.h"'));
	assert.match(schemaProvenance(paths).files.server.sha256, /^[0-9a-f]{64}$/);
});

test('schema apply refuses incompatible or partial source without mutation', t => {
	const paths = fixture(t);
	for (const source of ['unknown prepared source', '// LOCALAI_LLAMACPP_REQUEST_JSON_SCHEMA_HEALTH\n']) {
		writeFileSync(paths.server, source);
		assert.throws(() => applySchemaPatch(paths));
		assert.equal(readFileSync(paths.server, 'utf8'), source);
	}
});
