import type { components } from "../../../../api/generated/openapi";
import type {
  PackagedFactoryPublicDataSource,
  PackagedFactoryPublicExport,
} from "../public-contract";

type CatalogResponse = components["schemas"]["PackagedFactoryCatalogResponse"];

const manifestExport =
  "@you-agent-factory/packaged-factories/manifest" as const;
const schemaExport =
  "@you-agent-factory/packaged-factories/schemas/factory.json" as const;
const schemaIdentity =
  "https://schemas.portpowered.com/you/config/factory.schema.json";
const placeholderHash = "0".repeat(64);

function artifactExport(slug: string, format: "json" | "yaml") {
  return `@you-agent-factory/packaged-factories/factories/${slug}.${format}` as const;
}

function createValues(catalog: CatalogResponse) {
  const values = new Map<PackagedFactoryPublicExport, unknown>();
  values.set(schemaExport, {
    $id: schemaIdentity,
    $schema: "https://json-schema.org/draft/2020-12/schema",
  });
  values.set(manifestExport, {
    formatVersion: "1",
    factorySchema: schemaIdentity,
    factories: catalog.factories.map((factory) => ({
      name: factory.name,
      project: factory.project,
      slug: factory.slug,
      json: {
        locator: `generated/factories/${factory.slug}/factory.json`,
        sha256: placeholderHash,
      },
      yaml: {
        locator: `generated/factories/${factory.slug}/factory.yaml`,
        sha256: placeholderHash,
      },
    })),
  });
  for (const factory of catalog.factories) {
    values.set(artifactExport(factory.slug, "json"), factory.json);
    values.set(artifactExport(factory.slug, "yaml"), factory.yaml);
  }
  return values;
}

let catalogRequest: Promise<Map<PackagedFactoryPublicExport, unknown>> | undefined;

function loadValues() {
  catalogRequest ??= fetch("/packaged-factories")
    .then(async (response) => {
      if (!response.ok) {
        throw new Error(`packaged factory catalog request failed: ${response.status}`);
      }
      return createValues((await response.json()) as CatalogResponse);
    })
    .catch((error) => {
      catalogRequest = undefined;
      throw error;
    });
  return catalogRequest;
}

// The production source crosses the HTTP boundary. Tests inject a local source
// through AppProps and therefore do not need a running backend.
export const packagedFactoryPublicDataSource: PackagedFactoryPublicDataSource = {
  async read(specifier) {
    return (await loadValues()).get(specifier);
  },
};
