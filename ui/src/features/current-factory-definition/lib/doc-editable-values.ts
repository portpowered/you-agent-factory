import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { isFactoryBundledDocTargetPath } from "../../workflow-activity/lib/factory-bundled-docs";

export const FACTORY_DOCS_TARGET_PREFIX = "factory/docs/";

export interface EditableDocValues {
  fileName: string;
  inlineContent: string;
  targetPath: string;
}

export interface EditableDocDraft {
  fileName: string;
  inlineContent: string;
  originalExtension: string | null;
}

export function extractDocFileNameFromTargetPath(targetPath: string): string {
  const normalized = targetPath.replace(/\\/g, "/");
  if (!normalized.startsWith(FACTORY_DOCS_TARGET_PREFIX)) {
    return normalized;
  }

  return normalized.slice(FACTORY_DOCS_TARGET_PREFIX.length);
}

export function extractFileExtension(fileName: string): string | null {
  const lastDot = fileName.lastIndexOf(".");
  if (lastDot <= 0 || lastDot === fileName.length - 1) {
    return null;
  }

  return fileName.slice(lastDot);
}

export function resolveFileNameWithExtensionPreserved(
  fileName: string,
  originalExtension: string | null,
): string {
  const trimmed = fileName.trim();
  if (!originalExtension || trimmed.includes(".")) {
    return trimmed;
  }

  return `${trimmed}${originalExtension}`;
}

export function buildDocTargetPathFromFileName(fileName: string): string {
  const normalized = fileName.trim().replace(/\\/g, "/");
  const relative = normalized.startsWith(FACTORY_DOCS_TARGET_PREFIX)
    ? normalized.slice(FACTORY_DOCS_TARGET_PREFIX.length)
    : normalized;

  return `${FACTORY_DOCS_TARGET_PREFIX}${relative}`;
}

export function resolveDocTargetPathFromDraft(draft: EditableDocDraft): string {
  const resolvedFileName = resolveFileNameWithExtensionPreserved(
    draft.fileName,
    draft.originalExtension,
  );

  return buildDocTargetPathFromFileName(resolvedFileName);
}

export function resolveEditableDocValues(
  factory: CanonicalFactoryDefinition,
  targetPath: string,
): EditableDocValues | null {
  const bundledFile = factory.supportingFiles?.bundledFiles?.find(
    (candidate) =>
      candidate.type === "DOC" && candidate.targetPath === targetPath,
  );

  if (!bundledFile) {
    return null;
  }

  const fileName = extractDocFileNameFromTargetPath(targetPath);

  return {
    fileName,
    inlineContent: bundledFile.content?.inline ?? "",
    targetPath,
  };
}

export function editableDocDraftFromValues(
  values: EditableDocValues,
): EditableDocDraft {
  return {
    fileName: values.fileName,
    inlineContent: values.inlineContent,
    originalExtension: extractFileExtension(values.fileName),
  };
}

export function listFactoryDocTargetPaths(
  factory: CanonicalFactoryDefinition,
): string[] {
  return (factory.supportingFiles?.bundledFiles ?? [])
    .filter(
      (candidate) =>
        candidate.type === "DOC" &&
        candidate.targetPath != null &&
        isFactoryBundledDocTargetPath(candidate.targetPath),
    )
    .map((candidate) => candidate.targetPath as string);
}

export function applyEditableDocDraft(
  factory: CanonicalFactoryDefinition,
  originalTargetPath: string,
  draft: EditableDocDraft,
): CanonicalFactoryDefinition | null {
  const bundledFiles = factory.supportingFiles?.bundledFiles;
  if (!bundledFiles) {
    return null;
  }

  const docIndex = bundledFiles.findIndex(
    (candidate) =>
      candidate.type === "DOC" && candidate.targetPath === originalTargetPath,
  );
  if (docIndex < 0) {
    return null;
  }

  const existing = bundledFiles[docIndex];
  const nextTargetPath = resolveDocTargetPathFromDraft(draft);

  return {
    ...factory,
    supportingFiles: {
      ...factory.supportingFiles,
      bundledFiles: bundledFiles.map((file, index) =>
        index === docIndex
          ? {
              ...existing,
              content: {
                encoding: "utf-8",
                inline: draft.inlineContent,
              },
              targetPath: nextTargetPath,
              type: "DOC",
            }
          : file,
      ),
    },
  };
}
