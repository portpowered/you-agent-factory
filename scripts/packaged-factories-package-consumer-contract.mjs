import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile, realpath } from "node:fs/promises";
import { isAbsolute, relative, sep } from "node:path";

export const DIAGNOSTIC_PREFIX = "[packaged-factories-package-consumer]";

export function isWithin(parent, candidate) {
  const child = relative(parent, candidate);
  return (
    child === "" ||
    (!child.startsWith(`..${sep}`) && child !== ".." && !isAbsolute(child))
  );
}

function sha256(contents) {
  return createHash("sha256").update(contents).digest("hex");
}

export function requireObject(value, description) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${DIAGNOSTIC_PREFIX} ${description} must be an object`);
  }
  return value;
}

export function parseJSON(contents, description) {
  try {
    return JSON.parse(contents);
  } catch (error) {
    throw new Error(`${DIAGNOSTIC_PREFIX} ${description} is invalid JSON`, {
      cause: error,
    });
  }
}

export function selectLocalizedValue(asset, locale) {
  const value = requireObject(asset, "localized description");
  const exact = value.values?.[locale];
  if (typeof exact === "string" && exact.length > 0) {
    return exact;
  }
  if (typeof value.value === "string" && value.value.length > 0) {
    return value.value;
  }
  throw new Error(
    `${DIAGNOSTIC_PREFIX} localized description has no usable exact-locale or fallback value`,
  );
}

export function copyableInvocation(manifest) {
  for (const factory of manifest.factories ?? []) {
    for (const example of factory.examples ?? []) {
      if (
        example?.args &&
        typeof example.args === "object" &&
        !Array.isArray(example.args) &&
        Object.keys(example.args).length > 0
      ) {
        return {
          factory: factory.name,
          args: example.args,
        };
      }
    }
  }
  throw new Error(
    `${DIAGNOSTIC_PREFIX} installed manifest has no copyable invocation payload`,
  );
}

export async function resolvePublicArtifact({
  expectedPath,
  packageRoot,
  resolveSpecifier,
  specifier,
}) {
  let resolvedPath;
  try {
    resolvedPath = await resolveSpecifier(specifier);
  } catch (error) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} public export did not resolve: ${specifier}`,
      { cause: error },
    );
  }
  const canonicalPath = await realpath(resolvedPath).catch((error) => {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} public export target is missing: ${specifier}`,
      { cause: error },
    );
  });
  if (!isWithin(packageRoot, canonicalPath)) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} public export resolved outside installed package: ${specifier}`,
    );
  }
  const actualPath = relative(packageRoot, canonicalPath).replaceAll("\\", "/");
  if (actualPath !== expectedPath) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} public export resolved to ${actualPath}, want ${expectedPath}: ${specifier}`,
    );
  }
  return {
    contents: await readFile(canonicalPath),
    path: canonicalPath,
    specifier,
  };
}

function assertValidFactory(validate, factory, specifier) {
  if (validate(factory)) {
    return;
  }
  throw new Error(
    `${DIAGNOSTIC_PREFIX} Factory is invalid against installed schema: ${specifier}\n${JSON.stringify(validate.errors)}`,
  );
}

function assertFactoryPairEquivalent(jsonFactory, yamlFactory, slug) {
  try {
    assert.deepEqual(yamlFactory, jsonFactory);
  } catch (error) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} JSON/YAML Factory representations diverge: ${slug}`,
      { cause: error },
    );
  }
}

export async function verifyFactory({
  entry,
  packageName,
  packageRoot,
  parseYAML,
  resolveSpecifier,
  validate,
}) {
  const slug = entry?.slug;
  if (typeof slug !== "string" || slug.length === 0) {
    throw new Error(`${DIAGNOSTIC_PREFIX} manifest Factory slug is invalid`);
  }
  const jsonSpecifier = `${packageName}/factories/${slug}.json`;
  const yamlSpecifier = `${packageName}/factories/${slug}.yaml`;
  const jsonArtifact = await resolvePublicArtifact({
    expectedPath: entry.json?.locator,
    packageRoot,
    resolveSpecifier,
    specifier: jsonSpecifier,
  });
  const yamlArtifact = await resolvePublicArtifact({
    expectedPath: entry.yaml?.locator,
    packageRoot,
    resolveSpecifier,
    specifier: yamlSpecifier,
  });
  if (sha256(jsonArtifact.contents) !== entry.json?.sha256) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} manifest hash mismatch: ${jsonSpecifier}`,
    );
  }
  if (sha256(yamlArtifact.contents) !== entry.yaml?.sha256) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} manifest hash mismatch: ${yamlSpecifier}`,
    );
  }
  const jsonFactory = parseJSON(jsonArtifact.contents, jsonSpecifier);
  let yamlFactory;
  try {
    yamlFactory = parseYAML(yamlArtifact.contents.toString("utf8"));
  } catch (error) {
    throw new Error(
      `${DIAGNOSTIC_PREFIX} Factory is invalid YAML: ${yamlSpecifier}`,
      { cause: error },
    );
  }
  assertFactoryPairEquivalent(jsonFactory, yamlFactory, slug);
  assertValidFactory(validate, jsonFactory, jsonSpecifier);
  assertValidFactory(validate, yamlFactory, yamlSpecifier);
  return [jsonSpecifier, yamlSpecifier];
}
