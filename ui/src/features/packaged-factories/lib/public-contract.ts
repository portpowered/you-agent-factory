import Ajv2020, { type ValidateFunction } from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { parse as parseYaml } from "yaml";
import type {
  PackagedFactoryCatalogOutcome,
  PackagedFactoryLocalizableAsset,
  PackagedFactoryManifestEntry,
  PackagedFactoryManifestExample,
  PackagedFactoryPublicDataSource,
  PackagedFactoryPublicExport,
  PackagedFactorySelectionOutcome,
} from "./public-contract-types";

export type {
  PackagedFactoryCatalogOutcome,
  PackagedFactoryLocalizableAsset,
  PackagedFactoryManifestEntry,
  PackagedFactoryManifestExample,
  PackagedFactoryPublicDataSource,
  PackagedFactoryPublicExport,
  PackagedFactorySelectionOutcome,
  SelectedArtifactFailure,
  ValidatedPackagedFactoryManifest,
} from "./public-contract-types";

export const packagedFactoryManifestExport =
  "@you-agent-factory/packaged-factories/manifest" as const;
export const packagedFactorySchemaExport =
  "@you-agent-factory/packaged-factories/schemas/factory.json" as const;

const supportedFormatVersion = "1";
const supportedFactorySchemaIdentity =
  "https://schemas.portpowered.com/you/config/factory.schema.json";
const sha256Pattern = /^[0-9a-f]{64}$/;
const segmentPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const stableNamePattern = /^@you\/[a-z0-9]+(?:-[a-z0-9]+)*$/;

type InputRecord = Record<string, unknown>;

function isRecord(value: unknown): value is InputRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasExactKeys(
  value: InputRecord,
  required: readonly string[],
  optional: readonly string[] = [],
): boolean {
  const accepted = new Set([...required, ...optional]);
  return (
    required.every((key) => Object.hasOwn(value, key)) &&
    Object.keys(value).every((key) => accepted.has(key))
  );
}

function isArtifactMetadata(
  value: unknown,
  locator: string,
): value is { locator: string; sha256: string } {
  return (
    isRecord(value) &&
    hasExactKeys(value, ["locator", "sha256"]) &&
    value.locator === locator &&
    typeof value.sha256 === "string" &&
    sha256Pattern.test(value.sha256)
  );
}

function isCanonicalLocale(locale: string): boolean {
  if (
    locale.trim() !== locale ||
    locale.length === 0 ||
    locale.includes("--") ||
    !/^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$/.test(locale)
  ) {
    return false;
  }
  try {
    return Intl.getCanonicalLocales(locale)[0] === locale;
  } catch {
    return false;
  }
}

function isLocalizableAsset(
  value: unknown,
): value is PackagedFactoryLocalizableAsset {
  if (!isRecord(value) || typeof value.type !== "string") {
    return false;
  }
  if (
    !hasExactKeys(value, ["type", "value"], ["id", "locales", "values"]) ||
    value.type !== "LOCALIZABLE_ASSET" ||
    typeof value.value !== "string" ||
    value.value.trim().length === 0 ||
    (value.id !== undefined && typeof value.id !== "string")
  ) {
    return false;
  }
  if (
    value.locales !== undefined &&
    (!Array.isArray(value.locales) ||
      !value.locales.every(
        (locale): locale is string =>
          typeof locale === "string" && isCanonicalLocale(locale),
      ) ||
      new Set(value.locales).size !== value.locales.length)
  ) {
    return false;
  }
  return (
    value.values === undefined ||
    (isRecord(value.values) &&
      Object.entries(value.values).every(
        ([locale, localized]) =>
          isCanonicalLocale(locale) && typeof localized === "string",
      ))
  );
}

function isInvocationArguments(
  value: unknown,
): value is Readonly<Record<string, string | readonly string[]>> {
  return (
    isRecord(value) &&
    Object.values(value).every(
      (argument) =>
        typeof argument === "string" ||
        (Array.isArray(argument) &&
          argument.every((item) => typeof item === "string")),
    )
  );
}

function isExample(value: unknown): value is PackagedFactoryManifestExample {
  return (
    isRecord(value) &&
    hasExactKeys(value, ["name", "description", "args"]) &&
    typeof value.name === "string" &&
    value.name.trim() === value.name &&
    value.name.length > 0 &&
    isLocalizableAsset(value.description) &&
    isInvocationArguments(value.args)
  );
}

function parseManifestEntry(
  value: unknown,
): PackagedFactoryManifestEntry | undefined {
  if (
    !isRecord(value) ||
    !hasExactKeys(
      value,
      ["name", "project", "slug", "json", "yaml"],
      ["description", "examples"],
    ) ||
    typeof value.name !== "string" ||
    !stableNamePattern.test(value.name) ||
    typeof value.project !== "string" ||
    !segmentPattern.test(value.project) ||
    typeof value.slug !== "string" ||
    !segmentPattern.test(value.slug) ||
    value.name !== `@you/${value.slug}`
  ) {
    return undefined;
  }

  const jsonLocator = `generated/factories/${value.slug}/factory.json`;
  const yamlLocator = `generated/factories/${value.slug}/factory.yaml`;
  if (
    !isArtifactMetadata(value.json, jsonLocator) ||
    !isArtifactMetadata(value.yaml, yamlLocator) ||
    (value.description !== undefined &&
      !isLocalizableAsset(value.description)) ||
    (value.examples !== undefined &&
      (!Array.isArray(value.examples) || !value.examples.every(isExample)))
  ) {
    return undefined;
  }

  return value as unknown as PackagedFactoryManifestEntry;
}

function hasUniqueIdentities(
  entries: readonly PackagedFactoryManifestEntry[],
): boolean {
  const unique = (values: readonly string[]) =>
    new Set(values).size === values.length;
  return (
    unique(entries.map(({ name }) => name)) &&
    unique(entries.map(({ project }) => project)) &&
    unique(entries.map(({ slug }) => slug))
  );
}

function parseJsonData(input: unknown): unknown {
  if (typeof input === "string") {
    return JSON.parse(input);
  }
  return input;
}

async function readPublicExport(
  source: PackagedFactoryPublicDataSource,
  specifier: PackagedFactoryPublicExport,
): Promise<unknown | undefined> {
  try {
    return await source.read(specifier);
  } catch {
    return undefined;
  }
}

export async function loadPackagedFactoryCatalog(
  source: PackagedFactoryPublicDataSource,
): Promise<PackagedFactoryCatalogOutcome> {
  const input = await readPublicExport(source, packagedFactoryManifestExport);
  let manifest: unknown;
  try {
    manifest = parseJsonData(input);
  } catch {
    return { status: "invalid-contract" };
  }

  if (!isRecord(manifest)) {
    return { status: "invalid-contract" };
  }
  if (
    typeof manifest.formatVersion === "string" &&
    manifest.formatVersion !== supportedFormatVersion
  ) {
    return {
      status: "unsupported-version",
      formatVersion: manifest.formatVersion,
    };
  }
  if (
    !hasExactKeys(manifest, ["formatVersion", "factorySchema", "factories"]) ||
    manifest.formatVersion !== supportedFormatVersion ||
    manifest.factorySchema !== supportedFactorySchemaIdentity ||
    !Array.isArray(manifest.factories)
  ) {
    return { status: "invalid-contract" };
  }

  const entries = manifest.factories.map(parseManifestEntry);
  if (
    entries.some((entry) => entry === undefined) ||
    !hasUniqueIdentities(entries as PackagedFactoryManifestEntry[])
  ) {
    return { status: "invalid-contract" };
  }
  if (entries.length === 0) {
    return { status: "empty" };
  }

  return {
    status: "ready",
    manifest: {
      formatVersion: "1",
      factorySchema: manifest.factorySchema,
      factories: [...(entries as PackagedFactoryManifestEntry[])].sort(
        (left, right) =>
          left.name < right.name ? -1 : left.name > right.name ? 1 : 0,
      ),
    },
  };
}

function artifactExport(
  slug: string,
  format: "json" | "yaml",
): PackagedFactoryPublicExport {
  return `@you-agent-factory/packaged-factories/factories/${slug}.${format}`;
}

function createValidator(schema: unknown, schemaIdentity: string) {
  if (
    !isRecord(schema) ||
    schema.$id !== schemaIdentity ||
    schema.$schema !== "https://json-schema.org/draft/2020-12/schema"
  ) {
    return undefined;
  }
  try {
    const ajv = new Ajv2020({
      allErrors: true,
      coerceTypes: false,
      removeAdditional: false,
      strict: false,
      useDefaults: false,
    });
    addFormats(ajv);
    return ajv.compile(schema);
  } catch {
    return undefined;
  }
}

function canonicalJson(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map(canonicalJson).join(",")}]`;
  }
  if (isRecord(value)) {
    return `{${Object.keys(value)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalJson(value[key])}`)
      .join(",")}}`;
  }
  return JSON.stringify(value);
}

function parseArtifact(
  input: unknown,
  format: "json" | "yaml",
):
  | { readonly ok: true; readonly value: unknown; readonly text: string }
  | { readonly ok: false } {
  try {
    if (format === "json") {
      const value = parseJsonData(input);
      return {
        ok: true,
        value,
        text:
          typeof input === "string"
            ? input
            : JSON.stringify(value, undefined, 2),
      };
    }
    if (typeof input !== "string") {
      return { ok: false };
    }
    return { ok: true, value: parseYaml(input), text: input };
  } catch {
    return { ok: false };
  }
}

function validateArtifact(
  parsed: ReturnType<typeof parseArtifact>,
  format: "json" | "yaml",
  validate: ValidateFunction,
): PackagedFactorySelectionOutcome | undefined {
  if (!parsed.ok) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "parse-invalid", format },
    };
  }
  if (!validate(parsed.value)) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "schema-invalid", format },
    };
  }
  return undefined;
}

export async function resolvePackagedFactorySelection(
  source: PackagedFactoryPublicDataSource,
  catalog: Extract<PackagedFactoryCatalogOutcome, { status: "ready" }>,
  slug: string,
): Promise<PackagedFactorySelectionOutcome> {
  const entry = catalog.manifest.factories.find(
    (candidate) => candidate.slug === slug,
  );
  if (!entry) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "missing", format: "json" },
    };
  }

  const [schemaInput, jsonInput, yamlInput] = await Promise.all([
    readPublicExport(source, packagedFactorySchemaExport),
    readPublicExport(source, artifactExport(slug, "json")),
    readPublicExport(source, artifactExport(slug, "yaml")),
  ]);
  if (jsonInput === undefined) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "missing", format: "json" },
    };
  }
  if (yamlInput === undefined) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "missing", format: "yaml" },
    };
  }

  let schema: unknown;
  try {
    schema = parseJsonData(schemaInput);
  } catch {
    schema = undefined;
  }
  const validate = createValidator(schema, catalog.manifest.factorySchema);
  if (!validate) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "schema-invalid", format: "json" },
    };
  }

  const json = parseArtifact(jsonInput, "json");
  const yaml = parseArtifact(yamlInput, "yaml");
  const jsonFailure = validateArtifact(json, "json", validate);
  if (jsonFailure) {
    return jsonFailure;
  }
  const yamlFailure = validateArtifact(yaml, "yaml", validate);
  if (yamlFailure) {
    return yamlFailure;
  }
  if (
    !json.ok ||
    !yaml.ok ||
    canonicalJson(json.value) !== canonicalJson(yaml.value) ||
    !isRecord(json.value) ||
    json.value.name !== entry.slug ||
    json.value.id !== entry.project
  ) {
    return {
      status: "selected-artifact-failure",
      failure: { reason: "semantic-disagreement" },
    };
  }

  return {
    status: "ready",
    entry,
    json: json.value,
    yaml: yaml.value,
    jsonText: json.text,
    yamlText: yaml.text,
  };
}
