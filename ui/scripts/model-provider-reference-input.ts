import catalog from "@you-agent-factory/model-providers/catalog" with {
  type: "json",
};
import catalogSchema from "@you-agent-factory/model-providers/schemas/provider-catalog" with {
  type: "json",
};
import providerManifestSchema from "@you-agent-factory/model-providers/schemas/provider-manifest" with {
  type: "json",
};
import type {
  ProviderCatalog,
  ProviderManifest,
} from "@you-agent-factory/model-providers/types";
import Ajv2020, {
  type ErrorObject,
  type ValidateFunction,
} from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

type JsonSchema = Readonly<Record<string, unknown>>;

export type ProviderIndexInput = ProviderManifest & {
  readonly referencePath: `/reference/model-providers/${string}`;
};

export interface ProviderReferencePageInput {
  readonly manifest: ProviderManifest;
  readonly schema: JsonSchema;
}

export interface ModelProviderReferenceInput {
  readonly index: readonly ProviderIndexInput[];
  readonly providers: readonly ProviderReferencePageInput[];
}

export interface ModelProviderReferenceInputOptions {
  readonly catalog?: unknown;
  readonly catalogSchema?: JsonSchema;
  readonly providerManifestSchema?: JsonSchema;
}

function formatSchemaErrors(
  errors: readonly ErrorObject[] | null | undefined,
): string {
  return (errors ?? [])
    .map((error) => {
      const parameter =
        error.keyword === "required"
          ? `/${String(error.params.missingProperty)}`
          : "";
      return `${error.instancePath || "/"}${parameter} ${error.message ?? error.keyword}`;
    })
    .join("; ");
}

function compileValidator(schema: JsonSchema): ValidateFunction {
  const validator = new Ajv2020({
    allErrors: true,
    coerceTypes: false,
    removeAdditional: false,
    strict: false,
    useDefaults: false,
  });
  addFormats(validator);
  return validator.compile(schema);
}

function assertSchemaCompatible(
  validate: ValidateFunction,
  value: unknown,
  label: string,
): void {
  if (!validate(value)) {
    throw new Error(
      `[model-provider-reference-input] ${label} is schema-incompatible: ${formatSchemaErrors(validate.errors)}`,
    );
  }
}

export function buildModelProviderReferenceInput(
  options: ModelProviderReferenceInputOptions = {},
): ModelProviderReferenceInput {
  const catalogInput =
    options.catalog === undefined ? catalog : options.catalog;
  const catalogContract = options.catalogSchema ?? catalogSchema;
  const manifestContract =
    options.providerManifestSchema ?? providerManifestSchema;

  assertSchemaCompatible(
    compileValidator(catalogContract),
    catalogInput,
    "Provider Catalog",
  );

  const typedCatalog = catalogInput as ProviderCatalog;
  const validateManifest = compileValidator(manifestContract);
  for (const [index, provider] of typedCatalog.providers.entries()) {
    assertSchemaCompatible(
      validateManifest,
      provider,
      `Provider Manifest at providers[${index}]`,
    );
  }

  const providers = [...typedCatalog.providers].sort((left, right) => {
    if (left.id < right.id) return -1;
    if (left.id > right.id) return 1;
    return 0;
  });
  return {
    index: providers.map((provider) => ({
      ...provider,
      referencePath: `/reference/model-providers/${provider.id}`,
    })),
    providers: providers.map((manifest) => ({
      manifest,
      schema: manifestContract,
    })),
  };
}
