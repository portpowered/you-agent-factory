import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";

const FACTORY_DOCS_PREFIX = "factory/docs/";

export interface FactoryBundledDoc {
  displayLabel: string;
  nodeId: string;
  targetPath: string;
}

export type FactoryBundledDocFile = NonNullable<
  NonNullable<CanonicalFactoryDefinition["supportingFiles"]>["bundledFiles"]
>[number];

export function factoryBundledDocNodeId(targetPath: string): string {
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `doc:${targetPath}`;
}

export function factoryBundledDocDisplayLabel(targetPath: string): string {
  const normalizedPath = targetPath.replace(/\\/g, "/");
  const fileName = normalizedPath.slice(normalizedPath.lastIndexOf("/") + 1);
  return fileName.length > 0 ? fileName : normalizedPath;
}

export function isFactoryBundledDocTargetPath(targetPath: string): boolean {
  const normalizedPath = targetPath.replace(/\\/g, "/");
  return (
    normalizedPath.startsWith(FACTORY_DOCS_PREFIX) &&
    normalizedPath.length > FACTORY_DOCS_PREFIX.length
  );
}

export function listFactoryBundledDocs(
  factory: CanonicalFactoryDefinition | null | undefined,
): FactoryBundledDoc[] {
  const bundledFiles = factory?.supportingFiles?.bundledFiles ?? [];
  const docs: FactoryBundledDoc[] = [];

  for (const bundledFile of bundledFiles) {
    if (bundledFile.type !== "DOC") {
      continue;
    }

    const targetPath = bundledFile.targetPath?.trim();
    if (!targetPath || !isFactoryBundledDocTargetPath(targetPath)) {
      continue;
    }

    docs.push({
      displayLabel: factoryBundledDocDisplayLabel(targetPath),
      nodeId: factoryBundledDocNodeId(targetPath),
      targetPath,
    });
  }

  return docs.sort((left, right) =>
    left.targetPath.localeCompare(right.targetPath),
  );
}

export function findFactoryBundledDocFile(
  factory: CanonicalFactoryDefinition | null | undefined,
  targetPath: string,
): FactoryBundledDocFile | undefined {
  return (factory?.supportingFiles?.bundledFiles ?? []).find(
    (bundledFile) =>
      bundledFile.type === "DOC" && bundledFile.targetPath === targetPath,
  );
}

export function factoryBundledDocExists(
  factory: CanonicalFactoryDefinition | null | undefined,
  targetPath: string,
): boolean {
  return listFactoryBundledDocs(factory).some(
    (doc) => doc.targetPath === targetPath,
  );
}
