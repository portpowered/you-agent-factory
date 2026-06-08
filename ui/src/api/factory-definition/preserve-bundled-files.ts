import type { CanonicalFactoryDefinition } from "./api";

function bundledFilesFromFactory(
  factory: CanonicalFactoryDefinition | null | undefined,
) {
  return factory?.supportingFiles?.bundledFiles ?? [];
}

/**
 * Event-stream and timeline snapshot factories omit portable bundled files.
 * Keep the saved document's bundled files when the incoming factory does not
 * carry an explicit bundledFiles payload.
 */
export function preserveExistingBundledFilesWhenAbsent(
  incoming: CanonicalFactoryDefinition,
  existing: CanonicalFactoryDefinition | null | undefined,
): CanonicalFactoryDefinition {
  const existingBundledFiles = bundledFilesFromFactory(existing);
  if (existingBundledFiles.length === 0) {
    return incoming;
  }

  const incomingBundledFiles = incoming.supportingFiles?.bundledFiles;
  if (incomingBundledFiles !== undefined && incomingBundledFiles.length > 0) {
    return incoming;
  }

  return {
    ...incoming,
    supportingFiles: {
      ...incoming.supportingFiles,
      bundledFiles: existingBundledFiles,
    },
  };
}
