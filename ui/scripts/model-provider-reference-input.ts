import Ajv2020, {
  type ErrorObject,
  type ValidateFunction,
} from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import catalog from "../../packages/model-providers/generated/catalog.json" with {
  type: "json",
};
import catalogSchema from "../../packages/model-providers/generated/provider-catalog.schema.json" with {
  type: "json",
};
import providerManifestSchema from "../../packages/model-providers/generated/provider-manifest.schema.json" with {
  type: "json",
};
import type {
  ProviderCatalog,
  ProviderManifest,
} from "../../packages/model-providers/types/index.js";

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

function semanticError(message: string): never {
  throw new Error(
    `[model-provider-reference-input] Provider Catalog is semantically invalid: ${message}`,
  );
}

function assertCatalogSemantics(providers: readonly ProviderManifest[]): void {
  const ids = new Set<string>();
  for (const provider of providers) {
    if (ids.has(provider.id)) {
      semanticError(`duplicate canonical provider id "${provider.id}"`);
    }
    ids.add(provider.id);
  }

  const aliasOwners = new Map<string, string>();
  for (const provider of providers) {
    for (const alias of provider.aliases) {
      if (alias === provider.id) {
        semanticError(
          `provider "${provider.id}" alias "${alias}" duplicates its canonical id`,
        );
      }
      if (ids.has(alias)) {
        semanticError(
          `provider "${provider.id}" alias "${alias}" shadows a canonical provider id`,
        );
      }
      const owner = aliasOwners.get(alias);
      if (owner !== undefined) {
        semanticError(
          `provider alias "${alias}" is owned by both "${owner}" and "${provider.id}"`,
        );
      }
      aliasOwners.set(alias, provider.id);
    }
  }

  const deprecatedIds = new Set(
    providers
      .filter((provider) => provider.deprecation !== undefined)
      .map((provider) => provider.id),
  );
  for (const provider of providers) {
    assertProviderCapabilities(provider);
    const replacement = provider.deprecation?.replacementProviderId;
    if (replacement === undefined) continue;
    if (replacement === provider.id) {
      semanticError(
        `provider "${provider.id}" replacementProviderId cannot identify itself`,
      );
    }
    if (!ids.has(replacement)) {
      semanticError(
        `provider "${provider.id}" replacementProviderId "${replacement}" is not a canonical provider id`,
      );
    }
    if (deprecatedIds.has(replacement)) {
      semanticError(
        `provider "${provider.id}" replacementProviderId "${replacement}" is also deprecated`,
      );
    }
  }
}

function assertProviderCapabilities(provider: ProviderManifest): void {
  const execution = provider.maximumExecutionCapabilities;
  const fidelity = provider.maximumResponseFidelityCapabilities;
  const impossible: string[] = [];
  for (const field of [
    "messageDeltas",
    "toolOutputDeltas",
    "providerReconnect",
  ] as const) {
    if (fidelity[field] && !fidelity.nativeStreaming) {
      impossible.push(`${field} requires nativeStreaming`);
    }
  }
  if (fidelity.toolLifecycle && !execution.toolExecution) {
    impossible.push("toolLifecycle requires toolExecution");
  }
  if (fidelity.providerReconnect && !execution.sessionResume) {
    impossible.push("providerReconnect requires sessionResume");
  }
  if (impossible.length !== 0) {
    semanticError(
      `provider "${provider.id}" has impossible capabilities: ${impossible.sort().join(", ")}`,
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
  assertCatalogSemantics(typedCatalog.providers);

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
