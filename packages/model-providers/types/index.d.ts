// Code generated from provider-catalog.schema.json. DO NOT EDIT.

/** A customer-facing value with a required base fallback and optional exact locale overrides. Locale tags must use their canonical BCP 47 spelling. */
export type NameValue = {
  readonly id?: string;
  readonly locales?: ReadonlyArray<string>;
  readonly type: "LOCALIZABLE_ASSET";
  readonly value: string;
  readonly values?: Readonly<Record<string, string>>;
};

/** Coherent metadata for a deprecated provider entry. Presence of this object means the provider is deprecated. replacementProviderId, when present, must name a different canonical provider in the same catalog; it cannot identify the deprecated provider itself. */
export type ProviderDeprecation = {
  readonly deprecatedSince: string;
  readonly reason: NameValue;
  readonly replacementProviderId?: string;
};

/** Static endpoint transport kind that may be checked without credentials. */
export type ProviderDiscoveryEndpointKind = "local-http" | "remote-http" | "stdio" | "unix-socket";

/** Static, credential-free facts that tooling may use to explain how a provider can be discovered. Only names and endpoint kinds are published: credential values, environment values, endpoint addresses, machine-local paths, installation/readiness state, and pricing are outside this contract. */
export type ProviderDiscoveryPrerequisites = {
  readonly configurationKeys: ReadonlyArray<string>;
  readonly endpointKinds: ReadonlyArray<ProviderDiscoveryEndpointKind>;
  readonly executableNames: ReadonlyArray<string>;
};

/** One stable public documentation resource for a provider. */
export type ProviderDocumentationLink = {
  readonly kind: ProviderDocumentationLinkKind;
  readonly url: string;
};

/** Purpose of one stable public provider documentation link. */
export type ProviderDocumentationLinkKind = "homepage" | "setup" | "reference" | "support";

/** Maximum evidenced execution features of the provider integration. These values are independent of support posture and do not imply current-machine readiness. */
export type ProviderExecutionCapabilities = {
  readonly imageInput: boolean;
  readonly promptSubmission: boolean;
  readonly sessionResume: boolean;
  readonly structuredOutput: boolean;
  readonly toolExecution: boolean;
  readonly workingDirectory: boolean;
  readonly worktree: boolean;
};

/** How an implementation is supplied. Availability is publication metadata, not a live readiness or installation result. */
export type ProviderImplementationAvailability = "bundled" | "externally-supplied" | "catalog-only";

/** Public, data-only metadata for one model-provider integration. A manifest describes evidenced maximum behavior and publication posture; it never reports current-machine installation, authentication, readiness, pricing, or runtime registration. */
export type ProviderManifest = {
  readonly aliases: ReadonlyArray<string>;
  readonly deprecation?: ProviderDeprecation;
  readonly description: NameValue;
  readonly discovery: ProviderDiscoveryPrerequisites;
  readonly displayName: NameValue;
  readonly documentation: ReadonlyArray<ProviderDocumentationLink>;
  readonly id: string;
  readonly implementationAvailability: ProviderImplementationAvailability;
  readonly maximumExecutionCapabilities: ProviderExecutionCapabilities;
  readonly maximumResponseFidelityCapabilities: ProviderResponseFidelityCapabilities;
  readonly technicalSupportLevel: ProviderTechnicalSupportLevel;
};

/** Maximum evidenced response-event fidelity of the provider integration. Capabilities describe observable output independently of support posture. */
export type ProviderResponseFidelityCapabilities = {
  readonly fileChanges: boolean;
  readonly messageDeltas: boolean;
  readonly messageSnapshots: boolean;
  readonly nativeStreaming: boolean;
  readonly plans: boolean;
  readonly providerReconnect: boolean;
  readonly reasoningSummaries: boolean;
  readonly stableItemIds: boolean;
  readonly toolLifecycle: boolean;
  readonly toolOutputDeltas: boolean;
  readonly usage: boolean;
};

/** Maintainer-verified technical support posture for a provider integration. This value does not describe whether the provider is installed or ready on the current machine. */
export type ProviderTechnicalSupportLevel = "production" | "experimental" | "not-supported";

/** Versioned public collection of provider manifests. */
export type ProviderCatalog = {
  readonly formatVersion: "1.0.0";
  readonly providerSchema: "https://schemas.you.dev/model-providers/provider-manifest/1.0.0.schema.json";
  readonly providers: ReadonlyArray<ProviderManifest>;
  readonly publicationProvenance?: {
    readonly sourceCommit: string;
    readonly sourceRepository: string;
  };
};
