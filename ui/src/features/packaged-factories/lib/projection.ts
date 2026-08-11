import type {
  PackagedFactoryCatalogEntry,
  PackagedFactoryCatalogResponse,
} from "../../../api/packaged-factories";
import type {
  PackagedFactoryDescriptionViewModel,
  PackagedFactoryDetailViewModel,
  PackagedFactoryInventoryItemViewModel,
  PackagedFactoryInventoryViewModel,
  PackagedFactoryInvocationExamplesViewModel,
  PackagedFactoryInvocationExampleViewModel,
} from "./projection-types";

export type {
  PackagedFactoryConfigurationFormat,
  PackagedFactoryConfigurationViewModel,
  PackagedFactoryDescriptionViewModel,
  PackagedFactoryDetailViewModel,
  PackagedFactoryInventoryItemViewModel,
  PackagedFactoryInventoryViewModel,
  PackagedFactoryInvocationExamplesViewModel,
  PackagedFactoryInvocationExampleViewModel,
} from "./projection-types";

function projectDescription(
  asset: PackagedFactoryCatalogEntry["description"] | undefined,
  locale: string,
): PackagedFactoryDescriptionViewModel {
  if (!asset) {
    return { status: "unavailable" };
  }

  return {
    status: "available",
    value: asset.values?.[locale] ?? asset.value,
  };
}

function projectInventoryItem(
  entry: PackagedFactoryCatalogEntry,
  locale: string,
): PackagedFactoryInventoryItemViewModel {
  return {
    identity: entry.name,
    stableName: entry.name,
    project: entry.project,
    slug: entry.slug,
    description: projectDescription(entry.description, locale),
  };
}

export function projectPackagedFactoryInventory(
  catalog: PackagedFactoryCatalogResponse,
  locale: string,
): PackagedFactoryInventoryViewModel {
  const items = [...catalog.factories]
    .sort((left, right) =>
      left.name < right.name ? -1 : left.name > right.name ? 1 : 0,
    )
    .map((entry) => projectInventoryItem(entry, locale));

  return {
    items,
    byIdentity: Object.fromEntries(items.map((item) => [item.identity, item])),
  };
}

function cloneArguments(
  args: PackagedFactoryCatalogEntry["examples"][number]["args"],
): Readonly<Record<string, string | readonly string[]>> {
  return Object.fromEntries(
    Object.keys(args)
      .sort()
      .map((key) => {
        const value = args[key];
        return [key, Array.isArray(value) ? [...value] : value];
      }),
  );
}

function projectExample(
  stableName: string,
  example: PackagedFactoryCatalogEntry["examples"][number],
  locale: string,
): PackagedFactoryInvocationExampleViewModel {
  const args = cloneArguments(example.args);

  return {
    name: example.name,
    description: projectDescription(example.description, locale),
    args,
    copyValue: JSON.stringify({ factory: stableName, args }, undefined, 2),
  };
}

function projectExamples(
  entry: PackagedFactoryCatalogEntry,
  locale: string,
): PackagedFactoryInvocationExamplesViewModel {
  if (entry.examples.length === 0) {
    return { status: "none" };
  }

  return {
    status: "available",
    items: entry.examples.map((example) =>
      projectExample(entry.name, example, locale),
    ),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Projects one generated API entry and fails closed if an untyped fixture or
 * an invalid response ever reaches this boundary with a missing artifact.
 */
export function projectPackagedFactoryDetail(
  entry: PackagedFactoryCatalogEntry,
  locale: string,
): PackagedFactoryDetailViewModel | undefined {
  if (
    !isRecord(entry.json) ||
    typeof (entry as { readonly yaml?: unknown }).yaml !== "string" ||
    entry.yaml.trim().length === 0
  ) {
    return undefined;
  }

  const jsonText = JSON.stringify(entry.json, undefined, 2);
  if (jsonText === undefined) {
    return undefined;
  }

  return {
    identity: entry.name,
    stableName: entry.name,
    project: entry.project,
    slug: entry.slug,
    description: projectDescription(entry.description, locale),
    availableFormats: ["json", "yaml"],
    configurations: {
      json: {
        format: "json",
        displayValue: jsonText,
        copyValue: jsonText,
      },
      yaml: {
        format: "yaml",
        displayValue: entry.yaml,
        copyValue: entry.yaml,
      },
    },
    examples: projectExamples(entry, locale),
  };
}
