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
import type { CurrentActivityGraphEditorController } from "../hooks/current-activity-graph-state-value";

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

export interface CurrentActivityGraphSaveNotificationEffects {
  error: (
    title: string,
    options: { description: string; duration: number },
  ) => void;
  success: (
    title: string,
    options: { description: string; duration: number },
  ) => void;
  warning: (
    title: string,
    options: { description: string; duration: number },
  ) => void;
}

const defaultNotificationEffects: CurrentActivityGraphSaveNotificationEffects =
  {
    error: (title, options) => {
      toast.error(title, options);
    },
    success: (title, options) => {
      toast.success(title, options);
    },
    warning: (title, options) => {
      toast.warning(title, options);
    },
  };

export function CurrentActivityGraphSaveNotifications({
  editorController,
  locale,
  notificationEffects = defaultNotificationEffects,
}: {
  editorController: CurrentActivityGraphEditorController;
  notificationEffects?: CurrentActivityGraphSaveNotificationEffects;
  locale?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const lastDeliveredDeliveryKeyRef =
    useRef<SaveNotificationDeliveryKey | null>(null);
  const saveAttemptRevision = editorController.saveControls.attemptRevision;
  const saveMutationError = editorController.status.saveError;
  const notification = resolveGraphDocumentSaveToastNotification({
    documentSave: editorController.saveControls.feedback,
    hasDraftChanges: editorController.status.hasSharedGraphChanges,
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
      notificationEffects.warning(notification.title, {
        description: notification.description,
        duration: GLOBAL_TOAST_DURATION_MS,
      });
      return;
    }

    if (notification.kind === "error") {
      notificationEffects.error(notification.title, {
        ...buildSaveErrorToastOptions(notification.description),
      });
      return;
    }

    notificationEffects.success(notification.title, {
      ...buildSaveSuccessToastOptions(notification.description),
    });
  }, [
    notification,
    notificationEffects,
    saveAttemptRevision,
    saveMutationError,
  ]);

  return null;
}
