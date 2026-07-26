import { useEffect, useRef } from "react";
import { toast } from "sonner";

import type { CurrentFactoryDefinitionError } from "../../../../../api/current-factory-definition";
import {
  buildCurrentSelectionSaveSuccessStableIdentity,
  buildSaveErrorStableIdentity,
  buildSaveErrorToastOptions,
  buildSaveNotificationDeliveryKey,
  buildSaveSuccessToastOptions,
  type CurrentSelectionSaveEntityKind,
  GLOBAL_TOAST_DURATION_MS,
  type SaveNotificationDeliveryKey,
  type SaveNotificationStableIdentity,
  shouldDeliverSaveNotification,
} from "../../../../notifications/lib/save-notification-delivery-policy";
import type { FactoryDocumentSaveState } from "../../hooks/factory-document-save-types";
import {
  type CurrentSelectionSaveToastMessages,
  type CurrentSelectionSaveToastNotification,
  resolveCurrentSelectionSaveToastNotification,
} from "../../lib/current-selection-save-notifications";

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
  notification: CurrentSelectionSaveToastNotification,
  saveMutationError: unknown,
): SaveNotificationStableIdentity {
  if (notification.kind === "success") {
    return buildCurrentSelectionSaveSuccessStableIdentity(
      notification.entityKind,
    );
  }

  return buildSaveErrorStableIdentity({
    message: notification.key,
    code:
      notification.kind === "warning"
        ? (readSaveErrorCode(saveMutationError) ?? "warning")
        : readSaveErrorCode(saveMutationError),
  });
}

export type CurrentSelectionSaveNotificationsProps = {
  documentSave: FactoryDocumentSaveState;
  entityKind: CurrentSelectionSaveEntityKind;
  hasDraftChanges: boolean;
  /** Reserved for catalog resolution when wiring from the selection widget. */
  locale?: string;
  messages: CurrentSelectionSaveToastMessages;
  saveAttemptRevision: number;
  saveMutationError: Pick<
    CurrentFactoryDefinitionError,
    "code" | "message" | "targets"
  > | null;
};

export function CurrentSelectionSaveNotifications({
  documentSave,
  entityKind,
  hasDraftChanges,
  messages,
  saveAttemptRevision,
  saveMutationError,
}: CurrentSelectionSaveNotificationsProps) {
  const lastDeliveredDeliveryKeyRef =
    useRef<SaveNotificationDeliveryKey | null>(null);
  const notification = resolveCurrentSelectionSaveToastNotification({
    documentSave,
    entityKind,
    hasDraftChanges,
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
