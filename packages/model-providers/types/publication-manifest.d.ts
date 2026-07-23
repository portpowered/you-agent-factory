interface PublicationExport {
	readonly path: string;
	readonly family: "model-providers";
	readonly artifactHash: string;
	readonly documentation: Readonly<Record<string, unknown>>;
	readonly lifecycle: Readonly<Record<string, unknown>>;
}

interface PublicationManifest {
	readonly formatVersion: "1.0.0";
	readonly packageId: "you-agent-factory.model-providers";
	readonly packageVersion: string;
	readonly sourceCommit: string;
	readonly familyFormatVersions: Readonly<Record<"model-providers", "1.0.0">>;
	readonly exports: Readonly<Record<string, PublicationExport>>;
}

declare const manifest: PublicationManifest;
export = manifest;
