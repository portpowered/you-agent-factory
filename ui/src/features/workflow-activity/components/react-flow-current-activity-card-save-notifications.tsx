import { useEffect, useRef } from "react";
import { toast } from "sonner";

import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { GLOBAL_TOAST_DURATION_MS } from "../../notifications/components/app-notification-toaster";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";

export function CurrentActivityGraphSaveNotifications({
  editor,
  locale,
}: {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  locale?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const lastToastKeyRef = useRef<string | null>(null);
  const saveErrorMessage = editor.saveEditableDefinition.error?.message ?? null;
  const hasDraftChanges = editor.draftState.hasChanges;
  const showSaveSuccessToast =
    editor.graphDraftSaveSucceeded && !hasDraftChanges;

  useEffect(() => {
    const toastKey =
      saveErrorMessage !== null
        ? `error:${saveErrorMessage}`
        : showSaveSuccessToast
          ? "success"
          : null;

    if (toastKey === null || toastKey === lastToastKeyRef.current) {
      return;
    }

    lastToastKeyRef.current = toastKey;

    if (saveErrorMessage !== null) {
      toast.error(messages.noticeSaveFailedTitle, {
        description: saveErrorMessage,
        duration: GLOBAL_TOAST_DURATION_MS,
      });
      return;
    }

    toast.success(messages.noticeSaveSuccessTitle, {
      description: messages.noticeSaveSuccessDescription,
      duration: GLOBAL_TOAST_DURATION_MS,
    });
  }, [
    hasDraftChanges,
    messages.noticeSaveFailedTitle,
    messages.noticeSaveSuccessDescription,
    messages.noticeSaveSuccessTitle,
    saveErrorMessage,
    showSaveSuccessToast,
  ]);

  return null;
}
