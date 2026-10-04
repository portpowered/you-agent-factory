import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { compareBaselines, orphanTimingKeys, parseBaseline } from './lint-baseline-growth.mjs';

test('new rules seed once; established rules cannot grow even in a mixed seed', () => {
  assert.deepEqual(compareBaselines('', 'new|a|b\nnew|a|c'), []);
  assert.deepEqual(compareBaselines('old|a|b', 'old|a|b\nnew|a|c'), []);
  assert.deepEqual(compareBaselines('old|a|b', 'old|a|b\nold|a|c\nnew|a|c'), ['old|a|c']);
  assert.deepEqual(compareBaselines('new|a|b', 'new|a|b\nnew|a|c'), ['new|a|c']);
});

test('deletions, reordered keys, comments and empty baselines are permitted', () => {
  assert.deepEqual(compareBaselines('old|a|b\nold|a|c', '# comment\nold|a|c\n'), []);
  assert.deepEqual(compareBaselines('old|a|b', ''), []);
  assert.deepEqual(compareBaselines('', ''), []);
});

test('duplicate and malformed keys fail in either input', () => {
  for (const bad of ['bad', 'a||c', 'a| b|c', 'a|b|c|d', 'a|b|c\na|b|c',
    'testsleep-sleep-test|pkg/a|pkg/a/a_test.go::A::sleep::0',
    'testsleep-unknown|pkg/a|pkg/a/a_test.go::A::unknown::1']) {
    assert.throws(() => compareBaselines(bad, ''));
    assert.throws(() => compareBaselines('', bad));
  }
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
    '--base', 'missing-lint-baseline-file', '--head', 'internal/lint/analyzers/baseline.txt'], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.match(result.stderr, /ENOENT/u);
});
