import publishedCliManifest from "../../../../packages/api/generated/cli/commands.json" with {
  type: "json",
};

/** Raw package data stays unknown until the feature adapter validates it. */
export const publishedCliManifestArtifact: unknown = publishedCliManifest;
