// @vitest-environment node

import catalog from "@you-agent-factory/model-providers/catalog" with {
  type: "json",
};
import providerManifestSchema from "@you-agent-factory/model-providers/schemas/provider-manifest" with {
  type: "json",
};
import type {
  ProviderCatalog,
  ProviderManifest,
} from "@you-agent-factory/model-providers/types";
import { describe, expect, it } from "vitest";

import { buildModelProviderReferenceInput } from "./model-provider-reference-input";

function cloneCatalog(): ProviderCatalog {
  return structuredClone(catalog);
}

describe("buildModelProviderReferenceInput", () => {
  it("projects the packaged catalog into sorted index and schema-backed page input", () => {
    const input = buildModelProviderReferenceInput();
    const expectedIds = catalog.providers.map((provider) => provider.id).sort();

    expect(input.index.map((provider) => provider.id)).toEqual(expectedIds);
    expect(input.providers.map(({ manifest }) => manifest.id)).toEqual(
      expectedIds,
    );

    for (const [index, page] of input.providers.entries()) {
      const source = catalog.providers.find(
        (provider) => provider.id === page.manifest.id,
      );
      expect(page.manifest).toEqual(source);
      expect(input.index[index]).toEqual({
        ...source,
        referencePath: `/reference/model-providers/${page.manifest.id}`,
      });
      expect(page.schema).toBe(providerManifestSchema);
    }
  });

  it("includes a valid provider addition without a website inventory change", () => {
    const changedCatalog = cloneCatalog();
    const template = changedCatalog.providers.find(
      (provider) => provider.id === "codex",
    );
    expect(template).toBeDefined();

    const addedProvider: ProviderManifest = {
      ...structuredClone(template as ProviderManifest),
      aliases: [],
      description: {
        type: "LOCALIZABLE_ASSET",
        value: "A provider added through canonical catalog data.",
      },
      displayName: {
        type: "LOCALIZABLE_ASSET",
        value: "Zed",
      },
      id: "zed",
    };
    const catalogWithAddition: ProviderCatalog = {
      ...changedCatalog,
      providers: [addedProvider, ...changedCatalog.providers],
    };

    const input = buildModelProviderReferenceInput({
      catalog: catalogWithAddition,
    });

    expect(input.index.at(-1)).toEqual({
      ...addedProvider,
      referencePath: "/reference/model-providers/zed",
    });
    expect(input.providers.at(-1)?.manifest).toEqual(addedProvider);
  });

  it("fails with an actionable diagnostic for schema-incompatible data", () => {
    const invalidCatalog = cloneCatalog();
    const invalidProvider = {
      ...invalidCatalog.providers[0],
      technicalSupportLevel: "beta",
    };

    expect(() =>
      buildModelProviderReferenceInput({
        catalog: {
          ...invalidCatalog,
          providers: [invalidProvider, ...invalidCatalog.providers.slice(1)],
        },
      }),
    ).toThrow(
      /\[model-provider-reference-input\] Provider Catalog is schema-incompatible: .*technicalSupportLevel/,
    );
    expect(() => buildModelProviderReferenceInput({ catalog: null })).toThrow(
      "[model-provider-reference-input] Provider Catalog is schema-incompatible: / must be object",
    );
  });
});
