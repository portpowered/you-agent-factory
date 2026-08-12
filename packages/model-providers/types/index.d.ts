// Code generated from provider-catalog.schema.json. DO NOT EDIT.

/** A customer-facing value with a required base fallback and optional exact locale overrides. Locale tags must use their canonical BCP 47 spelling. */
export type NameValue = {
  readonly id?: string;
  readonly locales?: ReadonlyArray<string>;
  readonly type: "LOCALIZABLE_ASSET";
  readonly value: string;
  readonly values?: Readonly<Record<string, string>>;
};

/** Evidence state for delivering a resource through the ACP harness. */
export type ProviderACPResourceDelivery = "implemented" | "unsupported" | "conditional" | "unknown";

/** Typed ACP support metadata for a provider harness. */
export type ProviderACPSupport = ({
  readonly condition?: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly protocolVersion?: string;
  readonly resourceDelivery?: ProviderACPResourceDelivery;
  readonly support: ProviderCapabilitySupport;
}) & ({
  readonly condition: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly protocolVersion?: string;
  readonly resourceDelivery?: ProviderACPResourceDelivery;
  readonly support?: "conditional";
} | {
  readonly condition?: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly protocolVersion?: string;
  readonly resourceDelivery?: ProviderACPResourceDelivery;
  readonly support?: "supported" | "unsupported" | "unknown";
});

/** Bounded evidence record qualifying one or more published capability facts. */
export type ProviderCapabilityEvidence = {
  readonly factRefs?: ReadonlyArray<string>;
  readonly harnessVersion?: string;
  readonly id: string;
  readonly kind: ProviderCapabilityEvidenceKind;
  readonly url?: string;
  readonly verifiedOn: string;
};

/** Source class for evidence supporting a published capability fact. */
export type ProviderCapabilityEvidenceKind = "primary_documentation" | "protocol_probe" | "conformance_fixture" | "maintainer_assertion";

/** Evidence state shared by harness, modality, and tool capability facts. */
export type ProviderCapabilitySupport = "supported" | "unsupported" | "conditional" | "unknown";

/** Coherent metadata for a deprecated provider entry. Presence of this object means the provider is deprecated. replacementProviderId, when present, must name a different canonical provider in the same catalog; it cannot identify the deprecated provider itself. */
export type ProviderDeprecation = {
  readonly deprecatedSince: string;
  readonly reason: NameValue;
  readonly replacementProviderId?: string;
};

/** Static endpoint transport kind that may be checked without credentials. */
export type ProviderDiscoveryEndpointKind = "local-http" | "remote-http" | "stdio" | "unix-socket";

/** Sanitized prerequisite guidance that never carries a secret or machine-local value. */
export type ProviderDiscoveryPrerequisite = {
  readonly description: string;
  readonly kind: "executable" | "authentication" | "workspace" | "configuration";
  readonly name: string;
};

/** Static, credential-free facts that tooling may use to explain how a provider can be discovered. Only names and endpoint kinds are published: credential values, environment values, endpoint addresses, machine-local paths, installation/readiness state, and pricing are outside this contract. */
export type ProviderDiscoveryPrerequisites = {
  readonly configurationKeys: ReadonlyArray<string>;
  readonly endpointKinds: ReadonlyArray<ProviderDiscoveryEndpointKind>;
  readonly executableNames: ReadonlyArray<string>;
  readonly prerequisites?: ReadonlyArray<ProviderDiscoveryPrerequisite>;
};

/** One stable public documentation resource for a provider. */
export type ProviderDocumentationLink = {
  readonly kind: ProviderDocumentationLinkKind;
  readonly url: string;
};

/** Purpose of one stable public provider documentation link. */
export type ProviderDocumentationLinkKind = "homepage" | "setup" | "reference" | "support";

/** Provider-supported reasoning effort setting for one model. */
export type ProviderEffort = "minimal" | "low" | "medium" | "high" | "xhigh" | "max";

/** Maximum evidenced execution features of the provider integration. These values are independent of support posture and do not imply current-machine readiness. */
export type ProviderExecutionCapabilities = {
  readonly imageInput: boolean;
  readonly permissionBypass: boolean;
  readonly promptSubmission: boolean;
  readonly sessionResume: boolean;
  readonly structuredOutput: boolean;
  readonly toolExecution: boolean;
  readonly workingDirectory: boolean;
  readonly worktree: boolean;
};

/** Provider harness metadata, kept separate from model capability facts. */
export type ProviderHarness = {
  readonly acpSupport?: ProviderACPSupport;
  readonly kind: ProviderHarnessKind;
};

/** Execution harness family represented by a provider manifest. */
export type ProviderHarnessKind = "native_cli" | "acp";

/** How an implementation is supplied. Availability is publication metadata, not a live readiness or installation result. */
export type ProviderImplementationAvailability = "bundled" | "externally-supplied" | "catalog-only";

/** Named bounded provider constraint or documented behavior. */
export type ProviderKnownLimit = {
  readonly default?: number;
  readonly description: string;
  readonly kind: ProviderKnownLimitKind;
  readonly maximum?: number;
  readonly name: string;
  readonly unit: string;
  readonly value?: string;
};

/** Meaning of the value recorded by one named provider limit fact. */
export type ProviderKnownLimitKind = "maximum" | "default" | "behavior";

/** Public, data-only metadata for one model-provider integration. A manifest describes evidenced maximum behavior and publication posture; it never reports current-machine installation, authentication, readiness, pricing, or runtime registration. */
export type ProviderManifest = {
  readonly aliases: ReadonlyArray<string>;
  readonly deprecation?: ProviderDeprecation;
  readonly description: NameValue;
  readonly discovery: ProviderDiscoveryPrerequisites;
  readonly displayName: NameValue;
  readonly documentation: ReadonlyArray<ProviderDocumentationLink>;
  readonly evidence?: ReadonlyArray<ProviderCapabilityEvidence>;
  readonly harness?: ProviderHarness;
  readonly harnessRoutes?: ReadonlyArray<ProviderModality>;
  readonly id: string;
  readonly implementationAvailability: ProviderImplementationAvailability;
  readonly knownLimits?: ReadonlyArray<ProviderKnownLimit>;
  readonly maximumExecutionCapabilities: ProviderExecutionCapabilities;
  readonly maximumResponseFidelityCapabilities: ProviderResponseFidelityCapabilities;
  readonly modelCatalogPosture?: ProviderModelCatalogPosture;
  readonly models?: ReadonlyArray<ProviderModel>;
  readonly technicalSupportLevel: ProviderTechnicalSupportLevel;
  readonly tools?: ReadonlyArray<ProviderTool>;
};

/** Optional bounded media constraints for one modality route. */
export type ProviderMediaConstraints = {
  readonly maxBytes?: number;
  readonly maxDurationSeconds?: number;
  readonly maxItems?: number;
  readonly mediaTypes?: ReadonlyArray<string>;
};

/** One explicit directional modality fact for a harness route or model. */
export type ProviderModality = ({
  readonly condition?: string;
  readonly direction: ProviderModalityDirection;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly mediaConstraints?: ProviderMediaConstraints;
  readonly modality: ProviderModalityKind;
  readonly support: ProviderModalitySupport;
  readonly transport: ProviderModalityTransport;
}) & ({
  readonly condition: string;
  readonly direction?: ProviderModalityDirection;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly mediaConstraints?: ProviderMediaConstraints;
  readonly modality?: ProviderModalityKind;
  readonly support?: "conditional";
  readonly transport?: ProviderModalityTransport;
} | {
  readonly condition?: string;
  readonly direction?: ProviderModalityDirection;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly mediaConstraints?: ProviderMediaConstraints;
  readonly modality?: ProviderModalityKind;
  readonly support?: "supported" | "unsupported" | "unknown";
  readonly transport?: ProviderModalityTransport;
});

/** Direction in which a provider model accepts or emits a modality. */
export type ProviderModalityDirection = "input" | "output";

/** Media or content modality understood by a provider model. */
export type ProviderModalityKind = "text" | "image" | "audio" | "video";

/** Evidence state for a directional harness or model modality fact. */
export type ProviderModalitySupport = "supported" | "unsupported" | "conditional" | "unknown";

/** How a supported modality is supplied or returned. */
export type ProviderModalityTransport = "inline" | "file_path" | "acp_resource" | "tool_mediated" | "none";

/** Capability facts for one named model exposed by a provider. */
export type ProviderModel = {
  readonly efforts: ReadonlyArray<ProviderEffort>;
  readonly id: string;
  readonly modalities: ReadonlyArray<ProviderModality>;
};

/** How a provider's model identifiers are known to the published catalog. */
export type ProviderModelCatalogPosture = "exact" | "runtime_discovered" | "operator_selected" | "unknown";

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

/** One named provider tool fact used for execution planning. */
export type ProviderTool = ({
  readonly availability?: ProviderToolAvailability;
  readonly condition?: string;
  readonly defaultEnabled?: boolean | null;
  readonly description: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly name: string;
  readonly outputModalities?: ReadonlyArray<ProviderToolOutputModality>;
  readonly support: ProviderToolSupport;
}) & ({
  readonly availability?: ProviderToolAvailability;
  readonly condition: string;
  readonly defaultEnabled?: boolean | null;
  readonly description?: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly name?: string;
  readonly outputModalities?: ReadonlyArray<ProviderToolOutputModality>;
  readonly support?: "conditional";
} | {
  readonly availability?: ProviderToolAvailability;
  readonly condition?: string;
  readonly defaultEnabled?: boolean | null;
  readonly description?: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly name?: string;
  readonly outputModalities?: ReadonlyArray<ProviderToolOutputModality>;
  readonly support?: "supported" | "unsupported" | "unknown";
});

/** How a named tool becomes available to the provider harness. */
export type ProviderToolAvailability = "built_in" | "optional" | "operator_configured" | "external" | "unknown";

/** A modality produced by a tool, explicitly separate from direct model output. */
export type ProviderToolOutputModality = ({
  readonly condition?: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly mediaConstraints?: ProviderMediaConstraints;
  readonly modality: ProviderModalityKind;
  readonly support: ProviderCapabilitySupport;
  readonly transport: ProviderModalityTransport;
}) & ({
  readonly condition: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly mediaConstraints?: ProviderMediaConstraints;
  readonly modality?: ProviderModalityKind;
  readonly support?: "conditional";
  readonly transport?: ProviderModalityTransport;
} | {
  readonly condition?: string;
  readonly evidenceRefs?: ReadonlyArray<string>;
  readonly mediaConstraints?: ProviderMediaConstraints;
  readonly modality?: ProviderModalityKind;
  readonly support?: "supported" | "unsupported" | "unknown";
  readonly transport?: ProviderModalityTransport;
});

/** Evidence state for a named provider tool fact. */
export type ProviderToolSupport = "supported" | "unsupported" | "conditional" | "unknown";

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
