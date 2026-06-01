import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  buildGraphSaveErrorToastDescription,
} from "../../../factory-graph-editor/lib/graph-document-save-notifications";
import type { CurrentSelectionSaveEntityKind } from "../../../notifications/lib/save-notification-delivery-policy";
import type { FactoryDocumentSaveState } from "../hooks/factory-document-save-types";

export type { CurrentSelectionSaveEntityKind } from "../../../notifications/lib/save-notification-delivery-policy";

export type CurrentSelectionSaveToastKind = "success" | "error" | "warning";

export type CurrentSelectionSaveToastNotification = {
  description: string;
  entityKind: CurrentSelectionSaveEntityKind;
  key: string;
  kind: CurrentSelectionSaveToastKind;
  title: string;
};

export type CurrentSelectionSaveToastMessages = {
  saveFailedAffectedSummary?: (labels: string) => string;
  saveFailedTitle: string;
  saveSuccessDescription: string;
  saveSuccessTitle: string;
  staleVersionDetail: string;
};

export function resolveCurrentSelectionSaveToastNotification({
  documentSave,
  entityKind,
  hasDraftChanges,
  messages,
  saveMutationError,
}: {
  documentSave: FactoryDocumentSaveState;
  entityKind: CurrentSelectionSaveEntityKind;
  hasDraftChanges: boolean;
  messages: CurrentSelectionSaveToastMessages;
  saveMutationError: Pick<
    CurrentFactoryDefinitionError,
    "code" | "message" | "targets"
  > | null;
}): CurrentSelectionSaveToastNotification | null {
  if (documentSave.status === "success" && !hasDraftChanges) {
    return {
      description: messages.saveSuccessDescription,
      entityKind,
      key: "success",
      kind: "success",
      title: messages.saveSuccessTitle,
    };
  }

  if (shouldShowCurrentSelectionStaleSaveWarningToast(documentSave, saveMutationError)) {
    const warningMessage = resolveCurrentSelectionStaleSaveWarningMessage(
      documentSave,
      saveMutationError,
    );

    return {
      description: messages.staleVersionDetail,
      entityKind,
      key: `warning:${warningMessage}`,
      kind: "warning",
      title: warningMessage,
    };
  }

  if (documentSave.status === "error") {
    const description =
      messages.saveFailedAffectedSummary === undefined
        ? documentSave.errorMessage
        : buildGraphSaveErrorToastDescription(
            documentSave.errorMessage,
            saveMutationError?.targets,
            messages.saveFailedAffectedSummary,
          );

    return {
      description,
      entityKind,
      key: `error:${documentSave.errorMessage}:${description}`,
      kind: "error",
      title: messages.saveFailedTitle,
    };
  }

  return null;
}

function shouldShowCurrentSelectionStaleSaveWarningToast(
  documentSave: FactoryDocumentSaveState,
  saveMutationError: Pick<CurrentFactoryDefinitionError, "code"> | null,
): boolean {
  if (saveMutationError?.code === "STALE_FACTORY_VERSION") {
    return true;
  }

  return documentSave.status === "warning";
}

function resolveCurrentSelectionStaleSaveWarningMessage(
  documentSave: FactoryDocumentSaveState,
  saveMutationError: Pick<CurrentFactoryDefinitionError, "code" | "message"> | null,
): string {
  if (saveMutationError?.code === "STALE_FACTORY_VERSION") {
    return saveMutationError.message;
  }

  if (documentSave.status === "warning") {
    return documentSave.message;
  }

  return saveMutationError?.message ?? "";
}
