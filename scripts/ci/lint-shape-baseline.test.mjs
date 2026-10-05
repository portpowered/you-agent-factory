import assert from 'node:assert/strict';
import test from 'node:test';
import { collectTimingUnits, orphanTimingKeys } from './lint-baseline-growth.mjs';

test('shape debt requires retained compiler owners and exact files', () => {
  const keys = [
    'functional-test-missing-subsection|tests/functional/old|tests/functional/old/scenario_test.go',
    'deprecated-runtime-api-test|tests/functional/runtime_api_test|tests/functional/runtime_api/external_test.go#TestScenario',
    'service-root-exported-function|pkg/services/a|pkg/services/a/service.go#New',
    'service-root-unexpected-directory|pkg/services/a/extra|pkg/services/a/extra',
    'service-root-interface-count|pkg/services/b|<none>',
  ].join('\n');
  const live = 'm/tests/functional/old|scenario_test.go|||\nm/tests/functional/runtime_api_test [m/tests/functional/runtime_api.test]||||external_test.go\nm/pkg/services/a||||service.go\nm/pkg/services/a/extra||||helper.go\nm/pkg/services/b||||contract.go';
  assert.deepEqual(orphanTimingKeys(keys, live, 'm/'), []);
  assert.equal(orphanTimingKeys(keys, live.replaceAll('service.go', 'other.go'), 'm/').length, 1);
  assert.equal(orphanTimingKeys(keys, 'm/remaining||||file.go', 'm/').length, 5);
});

test('shape metadata uses compiler listing and propagates errors', () => {
  const key = 'deprecated-runtime-api-test|tests/functional/runtime_api_test|tests/functional/runtime_api/external_test.go#TestScenario';
  const calls = [];
  const metadata = collectTimingUnits(key, 'tags', 'go', (_go, args) => {
    calls.push(args);
    return { status: 0, stdout: 'github.com/portpowered/infinite-you/tests/functional/runtime_api_test [test]||||external_test.go' };
  }, 'windows');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].at(-1), './tests/functional/runtime_api');
  assert.deepEqual(orphanTimingKeys(key, metadata, 'github.com/portpowered/infinite-you/'), []);
  assert.throws(() => collectTimingUnits(key, 'tags', 'go', () => ({status: 1, stderr: 'unavailable compiler'})), /unavailable compiler/u);
});
