import {
  buildDocTargetPathFromFileName,
  listFactoryDocTargetPaths,
} from "../../current-factory-definition/lib/doc-editable-values";
import { isFactoryBundledDocTargetPath } from "../../workflow-activity/lib/factory-bundled-docs";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
} from "./draft/factory-graph-draft-types";
import {
  resolveDocTargetPathFromFileName,
  suggestDefaultDocFileName,
} from "./factory-graph-doc-editor";

export type FactoryGraphDocAddEntityDraft = {
  fileName: string;
  inlineContent: string;
  kind: "doc";
};

export type FactoryGraphDocAddEntityFieldErrors = {
  fileName?: string;
};

export function createFactoryGraphDocAddEntityDraft(
  factoryDefinition: CanonicalFactoryDefinition | null,
): FactoryGraphDocAddEntityDraft {
  return {
    fileName: suggestDefaultDocFileName(factoryDefinition),
    inlineContent: "",
    kind: "doc",
  };
}

export function validateFactoryGraphDocAddEntityDraft(
  draft: FactoryGraphDocAddEntityDraft,
  factoryDefinition: CanonicalFactoryDefinition | null,
): FactoryGraphDocAddEntityFieldErrors {
  const errors: FactoryGraphDocAddEntityFieldErrors = {};
  const fileName = draft.fileName.trim();

  if (fileName.length === 0) {
    errors.fileName = "Enter a file name before adding this doc.";
    return errors;
  }

  const targetPath = resolveDocTargetPathFromFileName(fileName);
  if (!isFactoryBundledDocTargetPath(targetPath)) {
    errors.fileName =
      "Doc file names must resolve to a path under factory/docs/.";
    return errors;
  }

  if (docTargetPathExists(targetPath, factoryDefinition)) {
    errors.fileName = `A doc at "${targetPath}" already exists in the draft.`;
  }

  return errors;
}

export function applyFactoryGraphDocAddEntityDraft(
  currentDraft: FactoryGraphDraft,
  entityDraft: FactoryGraphDocAddEntityDraft,
): FactoryGraphDraft {
  const nextDraft = structuredClone(currentDraft);
  nextDraft.additions.docs.push({
    inlineContent: entityDraft.inlineContent,
    targetPath: buildDocTargetPathFromFileName(entityDraft.fileName.trim()),
  });
  return nextDraft;
}

function docTargetPathExists(
  targetPath: string,
  factoryDefinition: CanonicalFactoryDefinition | null,
) {
  return listFactoryDocTargetPaths(
    factoryDefinition ?? { name: "Current Factory" },
  ).includes(targetPath);
}
