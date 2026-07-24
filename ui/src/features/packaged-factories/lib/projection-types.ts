export type PackagedFactoryDescriptionViewModel =
  | {
      readonly status: "available";
      readonly value: string;
    }
  | {
      readonly status: "unavailable";
    };

export interface PackagedFactoryInventoryItemViewModel {
  readonly identity: string;
  readonly stableName: string;
  readonly project: string;
  readonly slug: string;
  readonly description: PackagedFactoryDescriptionViewModel;
}

export interface PackagedFactoryInventoryViewModel {
  readonly items: readonly PackagedFactoryInventoryItemViewModel[];
  readonly byIdentity: Readonly<
    Record<string, PackagedFactoryInventoryItemViewModel>
  >;
}

export type PackagedFactoryConfigurationFormat = "json" | "yaml";

export interface PackagedFactoryConfigurationViewModel {
  readonly format: PackagedFactoryConfigurationFormat;
  readonly displayValue: string;
  readonly copyValue: string;
}

export interface PackagedFactoryInvocationExampleViewModel {
  readonly name: string;
  readonly description: PackagedFactoryDescriptionViewModel;
  readonly args: Readonly<Record<string, string | readonly string[]>>;
  readonly copyValue: string;
}

export type PackagedFactoryInvocationExamplesViewModel =
  | { readonly status: "none" }
  | {
      readonly status: "available";
      readonly items: readonly PackagedFactoryInvocationExampleViewModel[];
    };

export interface PackagedFactoryDetailViewModel {
  readonly identity: string;
  readonly stableName: string;
  readonly project: string;
  readonly slug: string;
  readonly description: PackagedFactoryDescriptionViewModel;
  readonly availableFormats: readonly PackagedFactoryConfigurationFormat[];
  readonly configurations: Readonly<
    Record<
      PackagedFactoryConfigurationFormat,
      PackagedFactoryConfigurationViewModel
    >
  >;
  readonly examples: PackagedFactoryInvocationExamplesViewModel;
}
