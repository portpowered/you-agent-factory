const assert = require('node:assert/strict');
const test = require('node:test');

const {
  REUSABLE_COMPONENT_SECTIONS,
  lintOpenAPIMainComponentsText,
} = require('./lint-openapi-main-components');
const {
  phaseDefinitions,
  runApiCommand,
  scriptPath,
} = require('./run-quiet-api-command');

test('accepts reusable components authored as single-object file references', () => {
  const sections = REUSABLE_COMPONENT_SECTIONS.map((section) => `  ${section}:\n${componentRefsForSection(section)}`).join('\n');

  assert.deepEqual(lintOpenAPIMainComponentsText(`openapi: 3.0.3\ncomponents:\n${sections}\n`), []);
});

test('rejects inline component bodies with section and component names', () => {
  const violations = lintOpenAPIMainComponentsText(`openapi: 3.0.3\ncomponents:\n  schemas:\n    SubmitWorkRequest:\n      type: object\n`, 'fixture.yaml');

  assert.equal(violations.length, 1);
  assert.equal(violations[0].section, 'schemas');
  assert.equal(violations[0].component, 'SubmitWorkRequest');
  assert.match(violations[0].message, /fixture\.yaml: components\.schemas\.SubmitWorkRequest missing \$ref/);
  assert.match(violations[0].message, /expected a single-object \$ref/);
});

test('rejects component references with sibling fields', () => {
  const violations = lintOpenAPIMainComponentsText(`openapi: 3.0.3\ncomponents:\n  responses:\n    BadRequest:\n      $ref: './components/responses/BadRequest.yaml'\n      description: Inline override\n`);

  assert.equal(violations.length, 1);
  assert.equal(violations[0].section, 'responses');
  assert.equal(violations[0].component, 'BadRequest');
  assert.match(violations[0].message, /must not include sibling fields beside \$ref/);
});

test('rejects internal reusable component references', () => {
  const violations = lintOpenAPIMainComponentsText(`openapi: 3.0.3\ncomponents:\n  schemas:\n    Foo:\n      $ref: '#/components/schemas/Bar'\n    Bar:\n      $ref: './components/schemas/Bar.yaml'\n`, 'fixture.yaml');

  assert.equal(violations.length, 1);
  assert.equal(violations[0].section, 'schemas');
  assert.equal(violations[0].component, 'Foo');
  assert.match(violations[0].message, /fixture\.yaml: components\.schemas\.Foo/);
  assert.match(violations[0].message, /must reference a component file under \.\/components\/ without an internal fragment/);
});

test('validate main runs component decomposition lint before redocly lint', () => {
  const specRoot = './api/openapi-main.yaml';
  const phases = phaseDefinitions(process.cwd(), process.platform, [specRoot]);

  assert.equal(phases['validate:main'].phases[0].command, process.execPath);
  assert.deepEqual(phases['validate:main'].phases[0].args, [scriptPath('lint-openapi-main-components.js'), specRoot]);
});

test('validate main stops before redocly when component decomposition lint fails', () => {
  const calls = [];
  const exitCode = runApiCommand('validate:main', {
    commandArgs: ['./fixture.yaml'],
    stdout: { write() {} },
    stderr: { write() {} },
    spawn: (command, args) => {
      calls.push([command, args]);
      return { status: calls.length === 1 ? 1 : 0 };
    },
  });

  assert.equal(exitCode, 1);
  assert.equal(calls.length, 1);
  assert.deepEqual(calls[0], [process.execPath, [scriptPath('lint-openapi-main-components.js'), './fixture.yaml']]);
});

function componentRefsForSection(section) {
  return `    ${section}Thing:\n      $ref: './components/${section}/${section}Thing.yaml'`;
}
