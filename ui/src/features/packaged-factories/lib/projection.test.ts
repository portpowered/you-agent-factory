import {
  projectPackagedFactoryDetail,
  projectPackagedFactoryInventory,
} from "./projection";
import type {
  PackagedFactoryCatalogOutcome,
  PackagedFactoryManifestEntry,
  PackagedFactorySelectionOutcome,
} from "./public-contract";

function entry(
  slug: string,
  overrides: Partial<PackagedFactoryManifestEntry> = {},
): PackagedFactoryManifestEntry {
  return {
    name: `@you/${slug}`,
    project: `builtin-${slug}`,
    slug,
    json: {
      locator: `generated/factories/${slug}/factory.json`,
      sha256: "a".repeat(64),
    },
    yaml: {
      locator: `generated/factories/${slug}/factory.yaml`,
      sha256: "b".repeat(64),
    },
    ...overrides,
  };
}

function catalog(
  factories: readonly PackagedFactoryManifestEntry[],
): Extract<PackagedFactoryCatalogOutcome, { status: "ready" }> {
  return {
    status: "ready",
    manifest: {
      formatVersion: "1",
      factorySchema:
        "https://schemas.portpowered.com/you/config/factory.schema.json",
      factories,
    },
  };
}

function selection(
  selectedEntry: PackagedFactoryManifestEntry,
): Extract<PackagedFactorySelectionOutcome, { status: "ready" }> {
  return {
    status: "ready",
    entry: selectedEntry,
    json: { id: selectedEntry.project, name: selectedEntry.slug },
    yaml: { id: selectedEntry.project, name: selectedEntry.slug },
    jsonText: `{\n  "id": "${selectedEntry.project}"\n}`,
    yamlText: `id: ${selectedEntry.project}\n`,
  };
}

describe("Packaged Factory inventory projection", () => {
  it("keys and orders disposable inventory by stable Factory identity", () => {
    const source = catalog([entry("zulu"), entry("alpha"), entry("middle")]);

    const projected = projectPackagedFactoryInventory(source, "en");

    expect(projected.items.map(({ identity }) => identity)).toEqual([
      "@you/alpha",
      "@you/middle",
      "@you/zulu",
    ]);
    expect(Object.keys(projected.byIdentity)).toEqual([
      "@you/alpha",
      "@you/middle",
      "@you/zulu",
    ]);
    expect(projected.byIdentity["@you/middle"]).toMatchObject({
      stableName: "@you/middle",
      project: "builtin-middle",
      slug: "middle",
    });
  });

  it("uses only an exact locale override and otherwise falls back to base copy", () => {
    const source = catalog([
      entry("localized", {
        description: {
          type: "LOCALIZABLE_ASSET",
          id: "factory.localized.description",
          value: "Base description",
          locales: ["en", "en-US"],
          values: {
            en: "English description",
            "en-US": "US description",
          },
        },
      }),
    ]);

    expect(
      projectPackagedFactoryInventory(source, "en-US").items[0]?.description,
    ).toEqual({ status: "available", value: "US description" });
    expect(
      projectPackagedFactoryInventory(source, "en-GB").items[0]?.description,
    ).toEqual({ status: "available", value: "Base description" });
    expect(
      projectPackagedFactoryInventory(source, "EN-us").items[0]?.description,
    ).toEqual({ status: "available", value: "Base description" });
  });

  it("represents a missing description without losing stable identity", () => {
    expect(
      projectPackagedFactoryInventory(catalog([entry("plain")]), "en").items[0],
    ).toEqual({
      identity: "@you/plain",
      stableName: "@you/plain",
      project: "builtin-plain",
      slug: "plain",
      description: { status: "unavailable" },
    });
  });
});

describe("Packaged Factory detail content projection", () => {
  it("projects localized examples, structured args, and deterministic copy data", () => {
    const selectedEntry = entry("hostile", {
      description: {
        type: "LOCALIZABLE_ASSET",
        value: "<strong>Base Factory</strong>",
        values: { "fr-CA": "<script>Factory française</script>" },
      },
      examples: [
        {
          name: "<img src=x onerror=alert(1)>",
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "{{ base example }}",
            values: { "fr-CA": "<b>Exemple</b>" },
          },
          args: {
            zeta: ["$(echo unsafe)", "<script>value</script>"],
            alpha: "{{template}}",
          },
        },
      ],
    });

    const detail = projectPackagedFactoryDetail(
      selection(selectedEntry),
      "fr-CA",
    );

    expect(detail).toMatchObject({
      identity: "@you/hostile",
      stableName: "@you/hostile",
      project: "builtin-hostile",
      description: {
        status: "available",
        value: "<script>Factory française</script>",
      },
      availableFormats: ["json", "yaml"],
      configurations: {
        json: {
          format: "json",
          displayValue: '{\n  "id": "builtin-hostile"\n}',
          copyValue: '{\n  "id": "builtin-hostile"\n}',
        },
        yaml: {
          format: "yaml",
          displayValue: "id: builtin-hostile\n",
          copyValue: "id: builtin-hostile\n",
        },
      },
      examples: {
        status: "available",
        items: [
          {
            name: "<img src=x onerror=alert(1)>",
            description: {
              status: "available",
              value: "<b>Exemple</b>",
            },
            args: {
              alpha: "{{template}}",
              zeta: ["$(echo unsafe)", "<script>value</script>"],
            },
          },
        ],
      },
    });
    if (detail.examples.status !== "available") {
      throw new Error("Expected projected invocation examples.");
    }
    expect(detail.examples.items[0]?.copyValue).toBe(
      [
        "{",
        '  "factory": "@you/hostile",',
        '  "args": {',
        '    "alpha": "{{template}}",',
        '    "zeta": [',
        '      "$(echo unsafe)",',
        '      "<script>value</script>"',
        "    ]",
        "  }",
        "}",
      ].join("\n"),
    );
  });
});

describe("Packaged Factory detail state projection", () => {
  it("uses explicit missing-description and no-examples states", () => {
    const detail = projectPackagedFactoryDetail(
      selection(entry("minimal")),
      "ja",
    );

    expect(detail.description).toEqual({ status: "unavailable" });
    expect(detail.examples).toEqual({ status: "none" });
    expect(detail.configurations.json.displayValue).toContain(
      "builtin-minimal",
    );
  });

  it("is reproducible and leaves canonical package data unchanged", () => {
    const selectedEntry = entry("stable", {
      examples: [
        {
          name: "default",
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "Base example",
          },
          args: { inputs: ["one", "two"], prompt: "hello" },
        },
      ],
    });
    const canonicalSelection = selection(selectedEntry);
    const before = structuredClone(canonicalSelection);

    const first = projectPackagedFactoryDetail(canonicalSelection, "en");
    const second = projectPackagedFactoryDetail(canonicalSelection, "en");

    expect(first).toEqual(second);
    expect(first).not.toBe(second);
    expect(first.examples).not.toBe(second.examples);
    expect(canonicalSelection).toEqual(before);
    if (
      first.examples.status !== "available" ||
      selectedEntry.examples === undefined
    ) {
      throw new Error("Expected projected and canonical invocation examples.");
    }
    expect(first.examples.items[0]?.args).not.toBe(
      selectedEntry.examples[0]?.args,
    );
    expect(first.examples.items[0]?.args.inputs).not.toBe(
      selectedEntry.examples[0]?.args.inputs,
    );
  });
});
