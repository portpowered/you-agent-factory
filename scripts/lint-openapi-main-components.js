#!/usr/bin/env node

const fs = require('node:fs');
const path = require('node:path');
const YAML = requireYaml();

const REUSABLE_COMPONENT_SECTIONS = [
  'schemas',
  'parameters',
  'responses',
  'examples',
  'headers',
  'requestBodies',
  'securitySchemes',
];

function requireYaml() {
  try {
    return require('yaml');
  } catch (error) {
    if (error && error.code !== 'MODULE_NOT_FOUND') {
      throw error;
    }
  }

  try {
    return require(path.join(__dirname, '..', 'ui', 'node_modules', 'yaml'));
  } catch (error) {
    if (error && error.code !== 'MODULE_NOT_FOUND') {
      throw error;
    }
    throw new Error('Cannot find the yaml package. Run `make ui-deps` before API validation.');
  }
}

function refValue(entry) {
  if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
    return undefined;
  }
  return entry.$ref;
}

function isSingleRefObject(entry) {
  return (
    entry &&
    typeof entry === 'object' &&
    !Array.isArray(entry) &&
    Object.keys(entry).length === 1 &&
    typeof entry.$ref === 'string' &&
    entry.$ref.length > 0
  );
}

function isComponentFileRef(ref) {
  return ref.startsWith('./components/') && !ref.includes('#');
}

function lintOpenAPIMainComponentsText(text, sourceName = 'api/openapi-main.yaml') {
  const document = YAML.parseDocument(text, { prettyErrors: false });
  if (document.errors.length > 0) {
    return document.errors.map((error) => ({
      section: '',
      component: '',
      message: `${sourceName}: ${error.message}`,
    }));
  }

  const spec = document.toJSON();
  const components = spec && typeof spec === 'object' ? spec.components : undefined;
  if (!components || typeof components !== 'object') {
    return [];
  }

  const violations = [];
  for (const section of REUSABLE_COMPONENT_SECTIONS) {
    const entries = components[section];
    if (!entries || typeof entries !== 'object' || Array.isArray(entries)) {
      continue;
    }

    for (const [component, entry] of Object.entries(entries)) {
      const ref = refValue(entry);
      if (isSingleRefObject(entry) && isComponentFileRef(ref)) {
        continue;
      }

      const detail = typeof ref === 'undefined'
        ? 'missing $ref'
        : isSingleRefObject(entry)
          ? 'must reference a component file under ./components/ without an internal fragment'
        : 'must not include sibling fields beside $ref';
      violations.push({
        section,
        component,
        message: `${sourceName}: components.${section}.${component} ${detail}; expected a single-object $ref to a component file because reusable components in api/openapi-main.yaml must be authored as file references only.`,
      });
    }
  }

  return violations;
}

function lintOpenAPIMainComponentsFile(specPath) {
  const text = fs.readFileSync(specPath, 'utf8');
  return lintOpenAPIMainComponentsText(text, specPath);
}

function main(argv = process.argv.slice(2)) {
  const specPath = argv[0] || 'api/openapi-main.yaml';
  const violations = lintOpenAPIMainComponentsFile(specPath);
  if (violations.length === 0) {
    return 0;
  }

  for (const violation of violations) {
    console.error(`[api:lint] ${violation.message}`);
  }
  return 1;
}

if (require.main === module) {
  process.exitCode = main();
}

module.exports = {
  REUSABLE_COMPONENT_SECTIONS,
  lintOpenAPIMainComponentsFile,
  lintOpenAPIMainComponentsText,
  main,
};
