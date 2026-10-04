import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { collectTimingUnits, orphanTimingKeys, parseBaseline } from './lint-baseline-growth.mjs';

test('duplicate and malformed ownership keys fail', () => {
  for (const bad of ['bad', 'a||c', 'a| b|c', 'a|b|c|d', 'a|b|c\na|b|c',
    'testsleep-sleep-test|pkg/a|pkg/a/a_test.go::A::sleep::0',
    'testsleep-unknown|pkg/a|pkg/a/a_test.go::A::unknown::1']) {
    assert.throws(() => parseBaseline(bad));
  }
});

test('landed Petri keys coexist with timing ownership validation', () => {
  const key = 'petri-public|pkg/a|Exported|pkg/b.Type';
  assert.equal(parseBaseline(key).size, 1);
  assert.deepEqual(orphanTimingKeys(key, '', 'm/'), []);
  assert.throws(() => parseBaseline('petri-public|pkg/a|Exported'));
});

test('compiler unit metadata preserves external test identities and rejects vanished owners', () => {
  const keys = 'testsleep-sleep-test|pkg/a_test|pkg/a/a_test.go::A::sleep::1';
  assert.equal(parseBaseline(keys).size, 1);
  assert.deepEqual(orphanTimingKeys(keys, 'm/pkg/a\nm/pkg/a_test [m/pkg/a.test]\nm/pkg/a.test', 'm/'), []);
  assert.deepEqual(orphanTimingKeys(keys, 'm/pkg/a', 'm/'), [keys]);
  const internal = keys.replace('pkg/a_test|', 'pkg/a|');
  assert.deepEqual(orphanTimingKeys(internal, 'm/pkg/a|||', 'm/'), [internal]);
  assert.deepEqual(orphanTimingKeys(internal, 'm/pkg/a|||a_test.go', 'm/'), []);
  assert.deepEqual(orphanTimingKeys(internal, 'm/pkg/a|||other_test.go', 'm/'), [internal]);
  assert.throws(() => orphanTimingKeys(keys, '', 'm/'));
});

test('command fails closed on unreadable input', () => {
  const result = spawnSync(process.execPath, ['scripts/ci/lint-baseline-growth.mjs',
    '--units', 'missing-lint-baseline-file', '--head', 'internal/lint/analyzers/baseline.txt'], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /ENOENT/u);
});

test('platform metadata resolves inactive external units and whole packages without accepting deleted owners', () => {
  const prefix = 'github.com/portpowered/infinite-you/';
  const external = 'testsleep-deadline-test|tests/process_test|tests/process/windows_test.go::Run::deadline::1';
  const inactive = 'testsleep-deadline-test|tests/omni|tests/omni/windows_test.go::Run::deadline::1';
  const deleted = 'testsleep-sleep-test|tests/gone|tests/gone/gone_test.go::Run::sleep::1';
  const head = [external, inactive, deleted].join('\n');
  const calls = [];
  const metadata = collectTimingUnits(head, 'tagunion', 'go', (_go, args, options) => {
    calls.push({ args, goos: options.env.GOOS });
    const stdout = options.env.GOOS === 'windows'
      ? `${prefix}tests/process||windows_test.go||\n${prefix}tests/process_test [${prefix}tests/process.test]||||windows_test.go\n${prefix}tests/omni|windows_test.go|||\n`
      : `${prefix}tests/process|||windows_test.go|\n`;
    return { status: 0, stdout };
  }, 'linux');
  assert.deepEqual(orphanTimingKeys(head, metadata, prefix), [deleted]);
  assert.deepEqual(calls.map(call => call.goos), ['linux', 'windows', 'darwin']);
  assert.deepEqual(calls.at(-1).args.slice(6), ['./tests/gone']);
  assert.ok(calls.every(call => !call.args.includes('./...')));
  assert.deepEqual(orphanTimingKeys(external, metadata.replaceAll('windows_test.go', 'other_test.go'), prefix), [external]);
});

test('metadata stops after resolution, preserves literal _test directories and fails on compiler errors', () => {
  const key = 'testsleep-sleep-test|tests/real_test|tests/real_test/a_test.go::Run::sleep::1';
  let calls = 0;
  collectTimingUnits(key, 'tagunion', 'go', (_go, args) => {
    calls++;
    assert.equal(args.at(-1), './tests/real_test');
    return { status: 0, stdout: 'github.com/portpowered/infinite-you/tests/real_test|a_test.go|||' };
  }, 'linux');
  assert.equal(calls, 1);
  assert.throws(() => collectTimingUnits(key, 'tagunion', 'go', () => ({ status: 1, stderr: 'bad metadata' }), 'linux'), /bad metadata/u);
  assert.equal(collectTimingUnits('', 'tagunion'), '');
  assert.deepEqual(orphanTimingKeys('', '', 'm/'), []);
});
