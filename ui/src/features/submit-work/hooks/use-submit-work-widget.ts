import { useMutation } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { isSubmitWorkAPIError, submitWork } from "../../../api/work";
import type { SubmitWorkMessages } from "../messages/submit-work";
import type {
  SubmitWorkDraft,
  SubmitWorkDraftItem,
  SubmitWorkDraftItemType,
  SubmitWorkStatus,
  SubmitWorkValidationErrors,
} from "../components/submit-work-card";

const DEFAULT_TEXT_ITEM_ID = "submission-item-1";

function createDefaultDraft(): SubmitWorkDraft {
  return {
    items: [createDefaultTextItem()],
    requestName: "",
    workTypeName: "",
  };
}

function createDefaultTextItem(): SubmitWorkDraftItem {
  return {
    id: DEFAULT_TEXT_ITEM_ID,
    text: "",
    type: "text",
  };
}

function createDraftItem(
  type: SubmitWorkDraftItemType,
  sequence: number,
): SubmitWorkDraftItem {
  const id = `submission-item-${sequence}`;

  if (type === "text") {
    return {
      id,
      text: "",
      type,
    };
  }

  return {
    id,
    type,
  };
}

const EMPTY_DRAFT: SubmitWorkDraft = createDefaultDraft();

function resetDraftPreservingWorkType(workTypeName: string): SubmitWorkDraft {
  return {
    ...createDefaultDraft(),
    workTypeName,
  };
}

function draftRequestText(draft: SubmitWorkDraft): string {
  return draft.items
    .filter((item): item is Extract<SubmitWorkDraftItem, { type: "text" }> => item.type === "text")
    .map((item) => item.text.trim())
    .filter((itemText) => itemText.length > 0)
    .join("\n\n");
}

const LEGACY_EMPTY_PAYLOAD = "";

export function useSubmitWorkWidget(
  sessionID: string,
  submitWorkTypes: DashboardSubmitWorkType[],
  messages: SubmitWorkMessages,
) {
  const [draft, setDraft] = useState<SubmitWorkDraft>(EMPTY_DRAFT);
  const [showValidation, setShowValidation] = useState(false);
  const nextItemSequenceRef = useRef(2);
  const submitWorkTypeNames = submitWorkTypes.map(
    (workType) => workType.work_type_name,
  );

  const mutation = useMutation({
    mutationFn: (request: Parameters<typeof submitWork>[0]) =>
      submitWork(request, { sessionID }),
    onSuccess: () => {
      setDraft((currentDraft) =>
        resetDraftPreservingWorkType(currentDraft.workTypeName),
      );
      setShowValidation(false);
    },
  });
  const resetMutation = mutation.reset;

  useEffect(() => {
    if (sessionID.trim().length === 0) {
      return;
    }
    nextItemSequenceRef.current = 2;
    setDraft(createDefaultDraft());
    setShowValidation(false);
    resetMutation();
  }, [resetMutation, sessionID]);

  useEffect(() => {
    if (
      draft.workTypeName.length > 0 &&
      !submitWorkTypeNames.includes(draft.workTypeName)
    ) {
      setDraft((currentDraft) => ({
        ...currentDraft,
        workTypeName: "",
      }));
    }
  }, [draft.workTypeName, submitWorkTypeNames]);

  const validationErrors = showValidation ? validateDraft(draft, messages) : {};

  return {
    draft,
    isSubmitting: mutation.isPending,
    onAddItem: (type: SubmitWorkDraftItemType) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      const nextItem = createDraftItem(type, nextItemSequenceRef.current);
      nextItemSequenceRef.current += 1;
      setDraft((currentDraft) => ({
        ...currentDraft,
        items: [...currentDraft.items, nextItem],
      }));
    },
    onItemTextChange: (itemId: string, value: string) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        items: currentDraft.items.map((item) =>
          item.id === itemId
            ? {
                ...item,
                text: value,
              }
            : item,
        ),
      }));
    },
    onRequestNameChange: (value: string) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        requestName: value,
      }));
    },
    onRemoveItem: (itemId: string) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      setDraft((currentDraft) => {
        const remainingItems = currentDraft.items.filter((item) => item.id !== itemId);

        return {
          ...currentDraft,
          items: remainingItems.length > 0 ? remainingItems : [createDefaultTextItem()],
        };
      });
    },
    onSubmit: () => {
      setShowValidation(true);
      mutation.reset();

      const nextValidationErrors = validateDraft(draft, messages);
      if (hasValidationErrors(nextValidationErrors)) {
        return;
      }

      mutation.mutate({
        name: draft.requestName,
        payload:
          draftRequestText(draft).trim().length === 0
            ? LEGACY_EMPTY_PAYLOAD
            : draftRequestText(draft),
        workTypeName: draft.workTypeName,
      });
    },
    onWorkTypeNameChange: (value: string) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        workTypeName: value,
      }));
    },
    status: buildStatus({
      draft,
      error: mutation.error,
      isSubmitting: mutation.isPending,
      isSuccess: mutation.isSuccess,
      messages,
      resultTraceID:
        mutation.data?.traceId ??
        (mutation.data as { trace_id?: string } | undefined)?.trace_id,
      showValidation,
      submitWorkTypeNames,
    }),
    submitWorkTypeNames,
    validationErrors,
  };
}

function buildStatus({
  draft,
  error,
  isSubmitting,
  isSuccess,
  messages,
  resultTraceID,
  showValidation,
  submitWorkTypeNames,
}: {
  draft: SubmitWorkDraft;
  error: unknown;
  isSubmitting: boolean;
  isSuccess: boolean;
  messages: SubmitWorkMessages;
  resultTraceID?: string;
  showValidation: boolean;
  submitWorkTypeNames: string[];
}): SubmitWorkStatus {
  if (isSubmitting) {
    return {
      kind: "submitting",
      message: messages.statusMessages.submitting,
    };
  }

  if (error) {
    return {
      kind: "error",
      message: submitWorkErrorMessage(error, messages),
    };
  }

  if (isSuccess) {
    return {
      kind: "success",
      message: messages.statusMessages.success(resultTraceID ?? "unavailable"),
    };
  }

  if (submitWorkTypeNames.length === 0) {
    return {
      kind: "guidance",
      message: messages.statusMessages.noWorkTypes,
    };
  }

  const validationErrors = validateDraft(draft, messages);
  if (showValidation && hasValidationErrors(validationErrors)) {
    return {
      kind: "validation-error",
      message: buildValidationSummary(validationErrors, messages),
    };
  }

  if (draft.workTypeName.length === 0) {
    if (draft.requestName.trim().length === 0) {
      return {
        kind: "guidance",
        message: messages.statusMessages.emptyGuidance,
      };
    }

    return {
      kind: "guidance",
      message: messages.statusMessages.workTypeOnly,
    };
  }

  if (draft.requestName.trim().length === 0) {
    return {
      kind: "guidance",
      message: messages.statusMessages.requestOnly,
    };
  }

  return {
    kind: "guidance",
    message: messages.statusMessages.ready,
  };
}

function buildValidationSummary(
  validationErrors: SubmitWorkValidationErrors,
  messages: SubmitWorkMessages,
): string {
  if (validationErrors.workTypeName) {
    if (validationErrors.requestName) {
      return messages.validationMessages.bothMissing;
    }
    return validationErrors.workTypeName;
  }
  if (validationErrors.requestName) {
    return validationErrors.requestName;
  }
  return messages.validationMessages.fallback;
}

function hasValidationErrors(
  validationErrors: SubmitWorkValidationErrors,
): boolean {
  return Boolean(validationErrors.requestName || validationErrors.workTypeName);
}

function submitWorkErrorMessage(
  error: unknown,
  messages: SubmitWorkMessages,
): string {
  if (isSubmitWorkAPIError(error) && error.message.length > 0) {
    return error.message;
  }
  return messages.statusMessages.errorFallback;
}

function validateDraft(
  draft: SubmitWorkDraft,
  messages: SubmitWorkMessages,
): SubmitWorkValidationErrors {
  const validationErrors: SubmitWorkValidationErrors = {};

  if (draft.workTypeName.length === 0) {
    validationErrors.workTypeName =
      messages.validationMessages.workTypeRequired;
  }
  if (draft.requestName.trim().length === 0) {
    validationErrors.requestName =
      messages.validationMessages.requestRequired;
  }
  return validationErrors;
}
