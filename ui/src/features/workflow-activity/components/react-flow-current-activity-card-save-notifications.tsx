import { useEffect, useRef } from "react";
import { toast } from "sonner";
import type { GraphDocumentSaveToastNotification } from "../../factory-graph-editor/lib/graph-document-save-notifications";
import { resolveGraphDocumentSaveToastNotification } from "../../factory-graph-editor/lib/graph-document-save-notifications";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  buildSaveErrorStableIdentity,
  buildSaveErrorToastOptions,
  buildSaveNotificationDeliveryKey,
  buildSaveSuccessStableIdentity,
  buildSaveSuccessToastOptions,
  GLOBAL_TOAST_DURATION_MS,
  type SaveNotificationDeliveryKey,
  type SaveNotificationStableIdentity,
  shouldDeliverSaveNotification,
} from "../../notifications/public";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";

function readSaveErrorCode(error: unknown): string | null {
  if (
    error !== null &&
    typeof error === "object" &&
    "code" in error &&
    typeof (error as { code: unknown }).code === "string"
  ) {
    return (error as { code: string }).code;
  }
  return null;
}

function buildToastStableIdentity(
  notification: GraphDocumentSaveToastNotification,
  saveMutationError: unknown,
): SaveNotificationStableIdentity {
  if (notification.kind === "success") {
    return buildSaveSuccessStableIdentity();
  }

  return buildSaveErrorStableIdentity({
    message: notification.key,
    code:
      notification.kind === "warning"
        ? (readSaveErrorCode(saveMutationError) ?? "warning")
        : readSaveErrorCode(saveMutationError),
  });
}

export function CurrentActivityGraphSaveNotifications({
  editor,
  locale,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  locale?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const lastDeliveredDeliveryKeyRef =
    useRef<SaveNotificationDeliveryKey | null>(null);
  const saveAttemptRevision = editor.saveAttemptRevision;
  const saveMutationError = editor.saveEditableDefinition.error ?? null;
  const notification = resolveGraphDocumentSaveToastNotification({
    documentSave: editor.documentSave,
    hasDraftChanges: editor.draftState.hasChanges,
    messages,
    saveMutationError,
  });

  useEffect(() => {
    if (notification === null) {
      return;
    }

    const deliveryKey = buildSaveNotificationDeliveryKey(
      buildToastStableIdentity(notification, saveMutationError),
      saveAttemptRevision,
    );

    if (
      !shouldDeliverSaveNotification(
        deliveryKey,
        lastDeliveredDeliveryKeyRef.current,
      )
    ) {
      return;
    }

    lastDeliveredDeliveryKeyRef.current = deliveryKey;

    if (notification.kind === "warning") {
      toast.warning(notification.title, {
        description: notification.description,
        duration: GLOBAL_TOAST_DURATION_MS,
      });
      return;
    }

    if (notification.kind === "error") {
      toast.error(notification.title, {
        ...buildSaveErrorToastOptions(notification.description),
      });
      return;
    }

    toast.success(notification.title, {
      ...buildSaveSuccessToastOptions(notification.description),
    });
  }, [notification, saveAttemptRevision, saveMutationError]);

  return null;
}
