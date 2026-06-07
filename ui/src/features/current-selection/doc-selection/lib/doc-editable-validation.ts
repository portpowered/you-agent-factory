import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  buildDocTargetPathFromFileName,
  type EditableDocDraft,
  listFactoryDocTargetPaths,
  resolveDocTargetPathFromDraft,
  resolveFileNameWithExtensionPreserved,
} from "../../../current-factory-definition/lib/doc-editable-values";
import { isFactoryBundledDocTargetPath } from "../../../workflow-activity/lib/factory-bundled-docs";
import type { DocDetailMessages } from "../messages/doc-detail-types";

export type EditableDocValidationField = "fileName" | "inlineContent";

export type EditableDocValidationErrors = Partial<
  Record<EditableDocValidationField, string>
>;

export interface ValidateEditableDocDraftContext {
  docTargetPaths: string[];
  originalTargetPath: string;
}

export function validateEditableDocDraft(
  draft: EditableDocDraft,
  messages: Pick<
    DocDetailMessages,
    | "editableConfigurationFileNameDotSegments"
    | "editableConfigurationFileNameDuplicate"
    | "editableConfigurationFileNameForwardSlashes"
    | "editableConfigurationFileNameInvalid"
    | "editableConfigurationFileNameMustBeFile"
    | "editableConfigurationFileNameOutsideDocsRoot"
    | "editableConfigurationFileNameRequired"
    | "editableConfigurationInlineContentRequired"
  >,
  context?: ValidateEditableDocDraftContext,
): EditableDocValidationErrors {
  const validationErrors: EditableDocValidationErrors = {};
  if (draft.fileName.trim().length === 0) {
    validationErrors.fileName = messages.editableConfigurationFileNameRequired;
  } else {
    const resolvedFileName = resolveFileNameWithExtensionPreserved(
      draft.fileName,
      draft.originalExtension,
    );
    const trimmedFileName = resolvedFileName.trim();
    const targetPath = buildDocTargetPathFromFileName(trimmedFileName);
    const pathError = validateDocTargetPath(targetPath, messages);
    if (pathError != null) {
      validationErrors.fileName = pathError;
    } else if (
      context &&
      targetPath !== context.originalTargetPath &&
      context.docTargetPaths.includes(targetPath)
    ) {
      validationErrors.fileName =
        messages.editableConfigurationFileNameDuplicate(trimmedFileName);
    }
  }

  if (draft.inlineContent.length === 0) {
    validationErrors.inlineContent =
      messages.editableConfigurationInlineContentRequired;
  }

  return validationErrors;
}

export function hasEditableDocValidationErrors(
  validationErrors: EditableDocValidationErrors,
): boolean {
  return Object.values(validationErrors).some(
    (message) => message != null && message.length > 0,
  );
}

export function mergeEditableDocContractValidationErrors(
  validationErrors: EditableDocValidationErrors,
  pendingFactoryDefinition: CanonicalFactoryDefinition | null,
): EditableDocValidationErrors {
  if (!pendingFactoryDefinition) {
    return validationErrors;
  }

  return validationErrors;
}

export function resolvePendingDocTargetPath(draft: EditableDocDraft): string {
  return resolveDocTargetPathFromDraft(draft);
}

function validateDocTargetPath(
  targetPath: string,
  messages: Pick<
    DocDetailMessages,
    | "editableConfigurationFileNameDotSegments"
    | "editableConfigurationFileNameForwardSlashes"
    | "editableConfigurationFileNameMustBeFile"
    | "editableConfigurationFileNameOutsideDocsRoot"
    | "editableConfigurationFileNameRequired"
  >,
): string | null {
  if (targetPath.includes("\\")) {
    return messages.editableConfigurationFileNameForwardSlashes;
  }

  if (!isFactoryBundledDocTargetPath(targetPath)) {
    return messages.editableConfigurationFileNameOutsideDocsRoot;
  }

  const segments = targetPath.split("/");
  if (segments.some((segment) => segment === "." || segment === "..")) {
    return messages.editableConfigurationFileNameDotSegments;
  }

  if (targetPath.endsWith("/")) {
    return messages.editableConfigurationFileNameMustBeFile;
  }

  const fileName = segments[segments.length - 1] ?? "";
  if (fileName.length === 0) {
    return messages.editableConfigurationFileNameRequired;
  }

  return null;
}

export function listEditableDocTargetPaths(
  factory: CanonicalFactoryDefinition,
): string[] {
  return listFactoryDocTargetPaths(factory);
}
