import { useEffect, useRef } from "react";
import { toast } from "sonner";
import type { GraphDocumentSaveToastNotification } from "../../factory-graph-editor/lib/document-save/graph-document-save-notifications";
import { resolveGraphDocumentSaveToastNotification } from "../../factory-graph-editor/lib/document-save/graph-document-save-notifications";
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
} from "../../notifications/lib/save-notification-delivery-policy";
import type { CurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";

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
  viewModel,
  locale,
}: {
  viewModel: CurrentActivityGraphCardViewModel;
  locale?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const lastDeliveredDeliveryKeyRef =
    useRef<SaveNotificationDeliveryKey | null>(null);
  const saveAttemptRevision = viewModel.saveControls.attemptRevision;
  const saveMutationError = viewModel.status.saveError;
  const notification = resolveGraphDocumentSaveToastNotification({
    documentSave: viewModel.saveControls.feedback,
    hasDraftChanges: viewModel.status.hasSharedGraphChanges,
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
