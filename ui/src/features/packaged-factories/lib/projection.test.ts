import type { PackagedFactoryCatalogEntry } from "../../../api/packaged-factories";
import {
  projectPackagedFactoryDetail,
  projectPackagedFactoryInventory,
} from "./projection";

function entry(
  slug: string,
  overrides: Partial<PackagedFactoryCatalogEntry> = {},
): PackagedFactoryCatalogEntry {
  return {
    name: `@you/${slug}`,
    project: `builtin-${slug}`,
    slug,
    description: {
      type: "LOCALIZABLE_ASSET",
      value: `${slug} description`,
    },
    examples: [
      {
        name: "default",
        description: {
          type: "LOCALIZABLE_ASSET",
          value: "Run the Factory.",
        },
        args: { input: "hello" },
      },
    ],
    json: { id: `builtin-${slug}`, name: slug },
    yaml: `id: builtin-${slug}\nname: ${slug}\n`,
    ...overrides,
  };
}

function catalog(factories: readonly PackagedFactoryCatalogEntry[]) {
  return { factories: [...factories] };
}

describe("Packaged Factory inventory projection", () => {
  it("keys and orders disposable inventory by stable Factory identity", () => {
    const projected = projectPackagedFactoryInventory(
      catalog([entry("zulu"), entry("alpha"), entry("middle")]),
      "en",
    );

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
    const missingDescription = {
      ...entry("plain"),
      description: undefined,
    } as unknown as PackagedFactoryCatalogEntry;

    expect(
      projectPackagedFactoryInventory(catalog([missingDescription]), "en")
        .items[0],
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
      json: { id: "builtin-hostile", name: "hostile" },
      yaml: "id: builtin-hostile\nname: hostile\n",
    });

    const detail = projectPackagedFactoryDetail(selectedEntry, "fr-CA");

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
          displayValue: '{\n  "id": "builtin-hostile",\n  "name": "hostile"\n}',
          copyValue: '{\n  "id": "builtin-hostile",\n  "name": "hostile"\n}',
        },
        yaml: {
          format: "yaml",
          displayValue: "id: builtin-hostile\nname: hostile\n",
          copyValue: "id: builtin-hostile\nname: hostile\n",
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
    if (detail?.examples.status !== "available") {
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
      {
        ...entry("minimal"),
        description: undefined,
        examples: [],
      } as unknown as PackagedFactoryCatalogEntry,
      "ja",
    );

    expect(detail?.description).toEqual({ status: "unavailable" });
    expect(detail?.examples).toEqual({ status: "none" });
    expect(detail?.configurations.json.displayValue).toContain(
      "builtin-minimal",
    );
  });

  it("fails closed for a selected entry with a missing artifact", () => {
    expect(
      projectPackagedFactoryDetail(
        { ...entry("broken"), yaml: "" } as PackagedFactoryCatalogEntry,
        "en",
      ),
    ).toBeUndefined();
  });

  it("is reproducible and leaves canonical API data unchanged", () => {
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
    const before = structuredClone(selectedEntry);

    const first = projectPackagedFactoryDetail(selectedEntry, "en");
    const second = projectPackagedFactoryDetail(selectedEntry, "en");

    expect(first).toEqual(second);
    expect(first).not.toBe(second);
    expect(first?.examples).not.toBe(second?.examples);
    expect(selectedEntry).toEqual(before);
    if (first?.examples.status !== "available") {
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
