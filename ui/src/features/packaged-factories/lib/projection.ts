import type {
  PackagedFactoryDescriptionViewModel,
  PackagedFactoryDetailViewModel,
  PackagedFactoryInventoryItemViewModel,
  PackagedFactoryInventoryViewModel,
  PackagedFactoryInvocationExamplesViewModel,
  PackagedFactoryInvocationExampleViewModel,
} from "./projection-types";
import type {
  PackagedFactoryCatalogOutcome,
  PackagedFactoryLocalizableAsset,
  PackagedFactoryManifestEntry,
  PackagedFactorySelectionOutcome,
} from "./public-contract";

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

type ReadyCatalog = Extract<
  PackagedFactoryCatalogOutcome,
  { readonly status: "ready" }
>;
type ReadySelection = Extract<
  PackagedFactorySelectionOutcome,
  { readonly status: "ready" }
>;

function projectDescription(
  asset: PackagedFactoryLocalizableAsset | undefined,
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
  entry: PackagedFactoryManifestEntry,
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
  catalog: ReadyCatalog,
  locale: string,
): PackagedFactoryInventoryViewModel {
  const items = [...catalog.manifest.factories]
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
  args: Readonly<Record<string, string | readonly string[]>>,
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
  example: NonNullable<PackagedFactoryManifestEntry["examples"]>[number],
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
  entry: PackagedFactoryManifestEntry,
  locale: string,
): PackagedFactoryInvocationExamplesViewModel {
  if (!entry.examples || entry.examples.length === 0) {
    return { status: "none" };
  }

  return {
    status: "available",
    items: entry.examples.map((example) =>
      projectExample(entry.name, example, locale),
    ),
  };
}

export function projectPackagedFactoryDetail(
  selection: ReadySelection,
  locale: string,
): PackagedFactoryDetailViewModel {
  const { entry } = selection;

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
        displayValue: selection.jsonText,
        copyValue: selection.jsonText,
      },
      yaml: {
        format: "yaml",
        displayValue: selection.yamlText,
        copyValue: selection.yamlText,
      },
    },
    examples: projectExamples(entry, locale),
  };
}
