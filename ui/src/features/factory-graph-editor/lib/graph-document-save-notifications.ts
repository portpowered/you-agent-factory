import type { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { FactoryValidationTarget } from "../../../api/factory-validation";
import type { FactoryDocumentSaveState } from "../../current-selection/base/hooks/factory-document-save-types";
import type { FactoryGraphEditorMessages } from "../messages/editor";
import { STALE_FACTORY_GRAPH_DRAFT_WARNING } from "./graph-document-save-state";

export type GraphDocumentSaveToastKind = "success" | "error" | "warning";

export type GraphDocumentSaveToastNotification = {
  description: string;
  key: string;
  kind: GraphDocumentSaveToastKind;
  title: string;
};

export function resolveGraphDocumentSaveToastNotification({
  documentSave,
  hasDraftChanges,
  messages,
  saveMutationError,
}: {
  documentSave: FactoryDocumentSaveState;
  hasDraftChanges: boolean;
  messages: Pick<
    FactoryGraphEditorMessages,
    | "noticeSaveFailedTitle"
    | "noticeSaveSuccessDescription"
    | "noticeSaveSuccessTitle"
    | "noticeStaleDescription"
    | "noticeStaleTitle"
  >;
  saveMutationError: Pick<
    CurrentFactoryDefinitionError,
    "code" | "message" | "targets"
  > | null;
}): GraphDocumentSaveToastNotification | null {
  if (documentSave.status === "success" && !hasDraftChanges) {
    return {
      description: messages.noticeSaveSuccessDescription,
      key: "success",
      kind: "success",
      title: messages.noticeSaveSuccessTitle,
    };
  }

  if (shouldShowGraphStaleSaveWarningToast(documentSave, saveMutationError)) {
    const warningMessage = resolveGraphStaleSaveWarningMessage(
      documentSave,
      saveMutationError,
    );

    return {
      description: buildStaleVersionToastDescription(
        warningMessage,
        messages.noticeStaleDescription,
      ),
      key: `warning:${warningMessage}`,
      kind: "warning",
      title: messages.noticeStaleTitle,
    };
  }

  if (documentSave.status === "error") {
    const description = buildGraphSaveErrorToastDescription(
      documentSave.errorMessage,
      saveMutationError?.targets,
    );

    return {
      description,
      key: `error:${documentSave.errorMessage}:${description}`,
      kind: "error",
      title: messages.noticeSaveFailedTitle,
    };
  }

  return null;
}

function shouldShowGraphStaleSaveWarningToast(
  documentSave: FactoryDocumentSaveState,
  saveMutationError: Pick<CurrentFactoryDefinitionError, "code"> | null,
): boolean {
  if (saveMutationError?.code === "STALE_FACTORY_VERSION") {
    return true;
  }

  if (documentSave.status !== "warning") {
    return false;
  }

  return documentSave.message !== STALE_FACTORY_GRAPH_DRAFT_WARNING;
}

function resolveGraphStaleSaveWarningMessage(
  documentSave: FactoryDocumentSaveState,
  saveMutationError: Pick<CurrentFactoryDefinitionError, "code" | "message"> | null,
): string {
  if (saveMutationError?.code === "STALE_FACTORY_VERSION") {
    return saveMutationError.message;
  }

  if (documentSave.status === "warning") {
    return documentSave.message;
  }

  return saveMutationError?.message ?? STALE_FACTORY_GRAPH_DRAFT_WARNING;
}

export function buildStaleVersionToastDescription(
  warningMessage: string,
  staleVersionDetail: string,
): string {
  return `${warningMessage}\n\n${staleVersionDetail}`;
}

export function buildGraphSaveErrorToastDescription(
  errorMessage: string,
  targets?: readonly FactoryValidationTarget[],
): string {
  const targetSummary = formatGraphSaveValidationTargetSummary(targets);
  if (targetSummary === null) {
    return errorMessage;
  }

  return `${errorMessage}\n\n${targetSummary}`;
}

export function formatGraphSaveValidationTargetSummary(
  targets?: readonly FactoryValidationTarget[],
): string | null {
  if (!targets?.length) {
    return null;
  }

  const labels = targets
    .map(formatGraphValidationTargetLabel)
    .filter((label) => label.length > 0);
  const uniqueLabels = [...new Set(labels)];

  if (uniqueLabels.length === 0) {
    return null;
  }

  return `Affected: ${uniqueLabels.join("; ")}`;
}

function formatGraphValidationTargetLabel(
  target: FactoryValidationTarget,
): string {
  const { id, location, type } = target.subject;
  const idSuffix = id.length > 0 ? ` ${id}` : "";
  return `${type}${idSuffix} (${location})`;
}
