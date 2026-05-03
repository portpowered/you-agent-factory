#!/usr/bin/env node

const { spawnSync } = require('node:child_process');

const GENERATED_PATHS = [
  'api/openapi.yaml',
  'pkg/api/generated/server.gen.go',
  'pkg/generatedclient/client.gen.go',
  'ui/src/api/generated/openapi.ts',
];

function runGitDiff(args) {
  return spawnSync('git', args, {
    encoding: 'utf8',
    shell: process.platform === 'win32',
    stdio: ['ignore', 'pipe', 'pipe'],
  });
}

function main() {
  const filteredStats = runGitDiff([
    'diff',
    '--ignore-blank-lines',
    '--numstat',
    '--',
    ...GENERATED_PATHS,
  ]);

  if (filteredStats.error) {
    console.error(`[api-smoke] Failed to inspect generated API drift: ${filteredStats.error.message}`);
    process.exitCode = 1;
    return;
  }

  if ((filteredStats.status ?? 1) > 1) {
    if (filteredStats.stdout) {
      process.stdout.write(filteredStats.stdout);
    }
    if (filteredStats.stderr) {
      process.stderr.write(filteredStats.stderr);
    }
    process.exitCode = filteredStats.status ?? 1;
    return;
  }

  const changedStats = filteredStats.stdout
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

  if (changedStats.length === 0) {
    process.exitCode = 0;
    return;
  }

  const fullDiff = runGitDiff(['diff', '--', ...GENERATED_PATHS]);
  if (fullDiff.stdout) {
    process.stdout.write(fullDiff.stdout);
  }
  if (fullDiff.stderr) {
    process.stderr.write(fullDiff.stderr);
  }

  console.error('[api-smoke] Generated API artifacts drifted.');
  process.exitCode = 1;
}

main();
