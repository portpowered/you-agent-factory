import { useEffect, useRef } from "react";
import { toast } from "sonner";

import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  buildSaveErrorStableIdentity,
  buildSaveErrorToastOptions,
  buildSaveNotificationDeliveryKey,
  buildSaveSuccessStableIdentity,
  buildSaveSuccessToastOptions,
  type SaveNotificationDeliveryKey,
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
  const saveError = editor.saveEditableDefinition.error ?? null;
  const saveErrorMessage = saveError?.message ?? null;
  const saveAttemptRevision = editor.saveAttemptRevision;
  const hasDraftChanges = editor.draftState.hasChanges;
  const showSaveSuccessToast =
    editor.graphDraftSaveSucceeded && !hasDraftChanges;

  useEffect(() => {
    let deliveryKey: SaveNotificationDeliveryKey | null = null;
    let emitError = false;
    let emitSuccess = false;

    if (saveErrorMessage !== null) {
      const identity = buildSaveErrorStableIdentity({
        message: saveErrorMessage,
        code: readSaveErrorCode(saveError),
      });
      deliveryKey = buildSaveNotificationDeliveryKey(
        identity,
        saveAttemptRevision,
      );
      emitError = true;
    } else if (showSaveSuccessToast) {
      const identity = buildSaveSuccessStableIdentity();
      deliveryKey = buildSaveNotificationDeliveryKey(
        identity,
        saveAttemptRevision,
      );
      emitSuccess = true;
    }

    if (
      deliveryKey === null ||
      !shouldDeliverSaveNotification(
        deliveryKey,
        lastDeliveredDeliveryKeyRef.current,
      )
    ) {
      return;
    }

    lastDeliveredDeliveryKeyRef.current = deliveryKey;

    if (emitError && saveErrorMessage !== null) {
      toast.error(messages.noticeSaveFailedTitle, {
        ...buildSaveErrorToastOptions(saveErrorMessage),
      });
      return;
    }

    if (emitSuccess) {
      toast.success(messages.noticeSaveSuccessTitle, {
        ...buildSaveSuccessToastOptions(messages.noticeSaveSuccessDescription),
      });
    }
  }, [
    messages.noticeSaveFailedTitle,
    messages.noticeSaveSuccessDescription,
    messages.noticeSaveSuccessTitle,
    saveAttemptRevision,
    saveError,
    saveErrorMessage,
    showSaveSuccessToast,
  ]);

  return null;
}
