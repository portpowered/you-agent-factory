import {
  buildDocTargetPathFromFileName,
  listFactoryDocTargetPaths,
} from "../../current-factory-definition/lib/doc-editable-values";
import {
  factoryBundledDocDisplayLabel,
  factoryBundledDocNodeId,
  isFactoryBundledDocTargetPath,
} from "../../workflow-activity/lib/factory-bundled-docs";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import { buildPendingFactoryDefinition } from "./draft/factory-graph-draft-apply";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
} from "./draft/factory-graph-draft-types";

export interface FactoryGraphDocRemovalIntent {
  confirmDescription: string;
  confirmLabel: string;
  ineligibleReason?: string;
  requiresConfirmation: boolean;
  targetPath: string;
  title: string;
}

// hardcoded-ui-copy-exception: non-product-diagnostic
const DEFAULT_DOC_FILE_NAME = "new-doc.md";

export function parseFactoryBundledDocNodeId(nodeId: string): string | null {
  if (!nodeId.startsWith("doc:")) {
    return null;
  }

  const targetPath = nodeId.slice("doc:".length);
  return isFactoryBundledDocTargetPath(targetPath) ? targetPath : null;
}

export function suggestDefaultDocFileName(
  factoryDefinition: CanonicalFactoryDefinition | null,
): string {
  const existingFileNames = new Set(
    listFactoryDocTargetPaths(
      factoryDefinition ?? { name: "Current Factory" },
    ).map((targetPath) => factoryBundledDocDisplayLabel(targetPath)),
  );

  if (!existingFileNames.has(DEFAULT_DOC_FILE_NAME)) {
    return DEFAULT_DOC_FILE_NAME;
  }

  const extensionIndex = DEFAULT_DOC_FILE_NAME.lastIndexOf(".");
  const stem = DEFAULT_DOC_FILE_NAME.slice(0, extensionIndex);
  const extension = DEFAULT_DOC_FILE_NAME.slice(extensionIndex);
  let suffix = 2;
  let candidate = `${stem}-${suffix}${extension}`;
  while (existingFileNames.has(candidate)) {
    suffix += 1;
    candidate = `${stem}-${suffix}${extension}`;
  }

  return candidate;
}

export function docTargetPathExists(
  targetPath: string,
  factoryDefinition: CanonicalFactoryDefinition | null,
): boolean {
  return listFactoryDocTargetPaths(
    factoryDefinition ?? { name: "Current Factory" },
  ).includes(targetPath);
}

export function applyFactoryGraphDocRemoval(
  currentDraft: FactoryGraphDraft,
  baseFactoryDefinition: CanonicalFactoryDefinition,
  targetPath: string,
): FactoryGraphDraft {
  const nextDraft = structuredClone(currentDraft);
  nextDraft.additions.docs = nextDraft.additions.docs.filter(
    (doc) => doc.targetPath !== targetPath,
  );

  if (docExistsInBaseDefinition(baseFactoryDefinition, targetPath)) {
    nextDraft.removals.docs = appendUnique(nextDraft.removals.docs, targetPath);
  }

  return nextDraft;
}

export function buildFactoryGraphDocRemovalIntent(options: {
  baseFactoryDefinition: CanonicalFactoryDefinition;
  draft: FactoryGraphDraft;
  locale?: string | null;
  nodeId: string;
}): FactoryGraphDocRemovalIntent | null {
  const targetPath = parseFactoryBundledDocNodeId(options.nodeId);
  if (!targetPath) {
    return null;
  }

  const messages = getFactoryGraphEditorMessages(options.locale);
  const currentFactoryDefinition =
    buildPendingFactoryDefinition(
      options.baseFactoryDefinition,
      options.draft,
    ) ?? options.baseFactoryDefinition;

  if (!docTargetPathExists(targetPath, currentFactoryDefinition)) {
    return null;
  }

  const displayLabel = factoryBundledDocDisplayLabel(targetPath);

  return {
    confirmDescription: messages.removalDocDescription(targetPath),
    confirmLabel: messages.removalDocConfirmLabel(displayLabel),
    requiresConfirmation: true,
    targetPath,
    title: messages.removalDocTitle(displayLabel),
  };
}

export function factoryGraphDocNodeIdForTargetPath(targetPath: string): string {
  return factoryBundledDocNodeId(targetPath);
}

function docExistsInBaseDefinition(
  factoryDefinition: CanonicalFactoryDefinition,
  targetPath: string,
): boolean {
  return (factoryDefinition.supportingFiles?.bundledFiles ?? []).some(
    (bundledFile) =>
      bundledFile.type === "DOC" && bundledFile.targetPath === targetPath,
  );
}

function appendUnique(items: string[], value: string): string[] {
  return items.includes(value) ? items : [...items, value];
}

export function resolveDocTargetPathFromFileName(fileName: string): string {
  return buildDocTargetPathFromFileName(fileName);
}
