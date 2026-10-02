import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { basename, dirname, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { pathToFileURL, fileURLToPath } from 'node:url';

const target = 'tools/grpc-server/grpc-server.cpp';
const markers = ['REQUEST_JSON_SCHEMA_FORWARD\n', 'REQUEST_JSON_SCHEMA_FORWARD_STREAM',
	'REQUEST_JSON_SCHEMA_FORWARD_PREDICT', 'REQUEST_JSON_SCHEMA_HEALTH'];

function git(root, arguments_) {
	return spawnSync('git', ['-C', root, 'apply', ...arguments_], { encoding: 'utf8' });
}

function checked(result, action) {
	if (result.status !== 0) throw new Error(`${action} failed: ${result.stderr || result.error || result.status}`);
}

export function extractSchemaHelper(source) {
	const start = 'static void llama_grpc_apply_request_json_schema(';
	if (source.split(start).length !== 2) throw new Error('expected exactly one compiled request schema helper');
	const offset = source.indexOf(start);
	const match = /\r?\n}\r?\n/.exec(source.slice(offset));
	if (!match) throw new Error('compiled request schema helper has no function boundary');
	return source.slice(offset, offset + match.index + match[0].length);
}

export function applySchemaPatch({ server, patch, testTemplate, testDestination, helperDestination }) {
	server = resolve(server);
	patch = resolve(patch);
	const root = dirname(dirname(dirname(server)));
	if (resolve(root, target) !== server) throw new Error(`server must be the prepared ${target}`);
	const patchText = readFileSync(patch, 'utf8').replace(/\r\n/g, '\n');
	if (patchText.split('diff --git ').length !== 2 || !patchText.includes(`+++ b/${target}\n`)) {
		throw new Error('schema patch must modify only the prepared gRPC server');
	}
	const source = readFileSync(server, 'utf8');
	const logical = source.replace(/\r\n/g, '\n');
	const counts = markers.map(marker => logical.split(`LOCALAI_LLAMACPP_${marker}`).length - 1);
	const applied = counts.every(count => count === 1);
	if (!applied && !counts.every(count => count === 0)) throw new Error('partially applied or duplicate schema markers');
	if (applied) checked(git(root, ['--reverse', '--check', patch]), 'schema reverse check');
	else {
		checked(git(root, ['--check', patch]), 'schema patch check');
		checked(git(root, [patch]), 'schema patch application');
		checked(git(root, ['--reverse', '--check', patch]), 'schema applied-byte verification');
	}
	const compiled = readFileSync(server, 'utf8');
	const helper = extractSchemaHelper(compiled);
	const header = '#pragma once\n#include <nlohmann/json.hpp>\n#include <stdexcept>\n#include <string>\n' +
		'#include "common/chat.h"\n' + helper;
	mkdirSync(dirname(resolve(helperDestination)), { recursive: true });
	mkdirSync(dirname(resolve(testDestination)), { recursive: true });
	writeFileSync(helperDestination, header);
	writeFileSync(testDestination, `#include "${basename(helperDestination)}"\n` + readFileSync(testTemplate, 'utf8'));
	return { status: applied ? 'already-applied' : 'applied', server, helperSha256: hash(helperDestination) };
}

function hash(path) { return createHash('sha256').update(readFileSync(path)).digest('hex'); }

export function schemaProvenance(paths) {
	return { patchID: 'localai-llamacpp-request-json-schema',
		nativeCommit: '6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef',
		files: Object.fromEntries(Object.entries(paths).map(([name, path]) => [name, { path: resolve(path), sha256: hash(path) }])) };
}

function main(arguments_) {
	const [command, ...rest] = arguments_;
	const options = {};
	for (let i = 0; i < rest.length; i += 2) {
		if (!rest[i].startsWith('--') || rest[i + 1] === undefined) throw new Error('expected named path arguments');
		options[rest[i].slice(2)] = rest[i + 1];
	}
	const paths = { server: options.server, patch: options.patch, testTemplate: options['test-template'],
		testDestination: options['test-destination'], helperDestination: options['helper-destination'] };
	if (Object.values(paths).some(value => !value)) throw new Error('server, patch, test-template, test-destination and helper-destination are required');
	if (command === 'apply') console.log(JSON.stringify(applySchemaPatch(paths)));
	else if (command === 'provenance') {
		if (!options.output) throw new Error('provenance output is required');
		writeFileSync(options.output, JSON.stringify(schemaProvenance({ ...paths, applicator: fileURLToPath(import.meta.url) }), null, 2) + '\n');
	} else throw new Error('expected apply or provenance');
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) main(process.argv.slice(2));
