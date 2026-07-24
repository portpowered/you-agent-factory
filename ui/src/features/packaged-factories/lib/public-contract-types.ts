export type PackagedFactoryPublicExport =
  | "@you-agent-factory/packaged-factories/manifest"
  | "@you-agent-factory/packaged-factories/schemas/factory.json"
  | `@you-agent-factory/packaged-factories/factories/${string}.json`
  | `@you-agent-factory/packaged-factories/factories/${string}.yaml`;

export interface PackagedFactoryPublicDataSource {
  read(specifier: PackagedFactoryPublicExport): Promise<unknown | undefined>;
}

export interface PackagedFactoryLocalizableAsset {
  readonly type: "LOCALIZABLE_ASSET";
  readonly value: string;
  readonly id?: string;
  readonly locales?: readonly string[];
  readonly values?: Readonly<Record<string, string>>;
}

export interface PackagedFactoryManifestExample {
  readonly name: string;
  readonly description: PackagedFactoryLocalizableAsset;
  readonly args: Readonly<Record<string, string | readonly string[]>>;
}

export interface PackagedFactoryManifestEntry {
  readonly name: string;
  readonly project: string;
  readonly slug: string;
  readonly json: Readonly<{ locator: string; sha256: string }>;
  readonly yaml: Readonly<{ locator: string; sha256: string }>;
  readonly description?: PackagedFactoryLocalizableAsset;
  readonly examples?: readonly PackagedFactoryManifestExample[];
}

export interface ValidatedPackagedFactoryManifest {
  readonly formatVersion: "1";
  readonly factorySchema: string;
  readonly factories: readonly PackagedFactoryManifestEntry[];
}

export type PackagedFactoryCatalogOutcome =
  | { readonly status: "empty" }
  | { readonly status: "invalid-contract" }
  | {
      readonly status: "ready";
      readonly manifest: ValidatedPackagedFactoryManifest;
    }
  | {
      readonly status: "unsupported-version";
      readonly formatVersion: string;
    };

export type SelectedArtifactFailure =
  | { readonly reason: "missing"; readonly format: "json" | "yaml" }
  | { readonly reason: "parse-invalid"; readonly format: "json" | "yaml" }
  | { readonly reason: "schema-invalid"; readonly format: "json" | "yaml" }
  | { readonly reason: "semantic-disagreement" };

export type PackagedFactorySelectionOutcome =
  | {
      readonly status: "ready";
      readonly entry: PackagedFactoryManifestEntry;
      readonly json: unknown;
      readonly yaml: unknown;
      readonly jsonText: string;
      readonly yamlText: string;
    }
  | {
      readonly status: "selected-artifact-failure";
      readonly failure: SelectedArtifactFailure;
    };
