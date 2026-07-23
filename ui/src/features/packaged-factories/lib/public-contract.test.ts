import {
  loadPackagedFactoryCatalog,
  type PackagedFactoryPublicDataSource,
  type PackagedFactoryPublicExport,
  packagedFactoryManifestExport,
  packagedFactorySchemaExport,
  resolvePackagedFactorySelection,
} from "./public-contract";

const schemaIdentity =
  "https://schemas.portpowered.com/you/config/factory.schema.json";
const validFactory = { id: "builtin-alpha", name: "alpha" };
const validSchema = {
  $id: schemaIdentity,
  $schema: "https://json-schema.org/draft/2020-12/schema",
  type: "object",
  additionalProperties: false,
  required: ["id", "name"],
  properties: {
    id: { type: "string" },
    name: { type: "string" },
  },
};

function entry(slug = "alpha", name = `@you/${slug}`) {
  return {
    name,
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
  };
}

function manifest(factories: unknown[] = [entry()]) {
  return {
    formatVersion: "1",
    factorySchema: schemaIdentity,
    factories,
  };
}

function source(
  overrides: Partial<Record<PackagedFactoryPublicExport, unknown>> = {},
) {
  const reads: PackagedFactoryPublicExport[] = [];
  const values: Partial<Record<PackagedFactoryPublicExport, unknown>> = {
    [packagedFactoryManifestExport]: manifest(),
    [packagedFactorySchemaExport]: validSchema,
    "@you-agent-factory/packaged-factories/factories/alpha.json":
      JSON.stringify(validFactory, undefined, 2),
    "@you-agent-factory/packaged-factories/factories/alpha.yaml":
      "id: builtin-alpha\nname: alpha\n",
    ...overrides,
  };
  const dataSource: PackagedFactoryPublicDataSource = {
    async read(specifier) {
      reads.push(specifier);
      return values[specifier];
    },
  };
  return { dataSource, reads };
}

async function readyCatalog(dataSource: PackagedFactoryPublicDataSource) {
  const result = await loadPackagedFactoryCatalog(dataSource);
  expect(result.status).toBe("ready");
  if (result.status !== "ready") {
    throw new Error("Expected a ready catalog.");
  }
  return result;
}

describe("Packaged Factory manifest validation", () => {
  it("validates unknown format 1 data and orders entries by stable name", async () => {
    const { dataSource, reads } = source({
      [packagedFactoryManifestExport]: JSON.stringify(
        manifest([entry("zulu"), entry("alpha")]),
      ),
    });

    const result = await loadPackagedFactoryCatalog(dataSource);

    expect(result).toMatchObject({
      status: "ready",
      manifest: {
        factories: [{ name: "@you/alpha" }, { name: "@you/zulu" }],
      },
    });
    expect(reads).toEqual([packagedFactoryManifestExport]);
  });

  it("returns an explicit empty outcome", async () => {
    const { dataSource } = source({
      [packagedFactoryManifestExport]: manifest([]),
    });
    await expect(loadPackagedFactoryCatalog(dataSource)).resolves.toEqual({
      status: "empty",
    });
  });

  it("distinguishes an unsupported version from an invalid contract", async () => {
    const unsupported = source({
      [packagedFactoryManifestExport]: {
        ...manifest(),
        formatVersion: "2",
      },
    });
    await expect(
      loadPackagedFactoryCatalog(unsupported.dataSource),
    ).resolves.toEqual({
      status: "unsupported-version",
      formatVersion: "2",
    });

    for (const invalid of [
      "{",
      { ...manifest(), extra: true },
      { ...manifest(), factorySchema: "https://example.com/factory.json" },
      manifest([entry(), entry()]),
      manifest([entry("alpha"), entry("alpha", "@you/other")]),
      manifest([entry("alpha", "@you/other")]),
      manifest([{ ...entry(), slug: "../alpha" }]),
      manifest([
        {
          ...entry(),
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "Alpha",
            values: { "en-us": "Alpha" },
          },
        },
      ]),
      manifest([
        {
          ...entry(),
          json: {
            ...entry().json,
            locator: "../generated/factory.json",
          },
        },
      ]),
    ]) {
      const { dataSource } = source({
        [packagedFactoryManifestExport]: invalid,
      });
      await expect(loadPackagedFactoryCatalog(dataSource)).resolves.toEqual({
        status: "invalid-contract",
      });
    }
  });

  it("accepts canonical localized metadata and treats source failures as invalid", async () => {
    const { dataSource } = source({
      [packagedFactoryManifestExport]: manifest([
        {
          ...entry(),
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "Alpha",
            locales: ["en-US"],
            values: { "fr-CA": "Alpha en français" },
          },
        },
      ]),
    });
    await expect(loadPackagedFactoryCatalog(dataSource)).resolves.toMatchObject(
      {
        status: "ready",
      },
    );

    await expect(
      loadPackagedFactoryCatalog({
        async read() {
          throw new Error("package unavailable");
        },
      }),
    ).resolves.toEqual({ status: "invalid-contract" });
  });
});

describe("Packaged Factory public artifact resolution", () => {
  it("resolves and validates both artifacts using public export identities only", async () => {
    const { dataSource, reads } = source();
    const catalog = await readyCatalog(dataSource);

    await expect(
      resolvePackagedFactorySelection(dataSource, catalog, "alpha"),
    ).resolves.toMatchObject({
      status: "ready",
      entry: { slug: "alpha" },
      json: validFactory,
      yaml: validFactory,
    });
    expect(reads).toEqual([
      packagedFactoryManifestExport,
      packagedFactorySchemaExport,
      "@you-agent-factory/packaged-factories/factories/alpha.json",
      "@you-agent-factory/packaged-factories/factories/alpha.yaml",
    ]);
  });
});

describe("Packaged Factory selected artifact failures", () => {
  it.each([
    [
      "invalid schema",
      {
        [packagedFactorySchemaExport]: {
          ...validSchema,
          $id: "https://example.com/factory.json",
        },
      },
      { reason: "schema-invalid", format: "json" },
    ],
    [
      "missing JSON",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.json": undefined,
      },
      { reason: "missing", format: "json" },
    ],
    [
      "missing YAML",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.yaml": undefined,
      },
      { reason: "missing", format: "yaml" },
    ],
    [
      "invalid JSON syntax",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.json": "{",
      },
      { reason: "parse-invalid", format: "json" },
    ],
    [
      "invalid YAML syntax",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.yaml": "[",
      },
      { reason: "parse-invalid", format: "yaml" },
    ],
    [
      "JSON schema failure",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.json":
          '{"name":"alpha"}',
      },
      { reason: "schema-invalid", format: "json" },
    ],
    [
      "YAML schema failure",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.yaml":
          "name: alpha\n",
      },
      { reason: "schema-invalid", format: "yaml" },
    ],
    [
      "semantic disagreement",
      {
        "@you-agent-factory/packaged-factories/factories/alpha.yaml":
          "id: builtin-other\nname: alpha\n",
      },
      { reason: "semantic-disagreement" },
    ],
  ])(
    "reports %s without yielding stale detail",
    async (_name, values, failure) => {
      const { dataSource } = source(values);
      const catalog = await readyCatalog(dataSource);

      await expect(
        resolvePackagedFactorySelection(dataSource, catalog, "alpha"),
      ).resolves.toEqual({
        status: "selected-artifact-failure",
        failure,
      });
    },
  );

  it("rejects a schema-valid pair whose identity disagrees with the manifest", async () => {
    const { dataSource } = source({
      "@you-agent-factory/packaged-factories/factories/alpha.json":
        '{"id":"builtin-other","name":"other"}',
      "@you-agent-factory/packaged-factories/factories/alpha.yaml":
        "id: builtin-other\nname: other\n",
    });
    const catalog = await readyCatalog(dataSource);

    await expect(
      resolvePackagedFactorySelection(dataSource, catalog, "alpha"),
    ).resolves.toEqual({
      status: "selected-artifact-failure",
      failure: { reason: "semantic-disagreement" },
    });
  });
});
