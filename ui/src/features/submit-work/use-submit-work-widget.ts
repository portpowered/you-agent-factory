import { useMutation } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { DashboardSubmitWorkType } from "../../api/dashboard/types";
import {
  isSubmitWorkAPIError,
  submitWork,
} from "../../api/work";
import type {
  SubmitWorkDraft,
  SubmitWorkStatus,
  SubmitWorkValidationErrors,
} from "./submit-work-card";
import type { SubmitWorkMessages } from "./messages/submit-work";

const EMPTY_DRAFT: SubmitWorkDraft = {
  requestName: "",
  requestText: "",
  workTypeName: "",
};

export function useSubmitWorkWidget(
  submitWorkTypes: DashboardSubmitWorkType[],
  messages: SubmitWorkMessages,
) {
  const [draft, setDraft] = useState<SubmitWorkDraft>(EMPTY_DRAFT);
  const [showValidation, setShowValidation] = useState(false);
  const submitWorkTypeNames = submitWorkTypes.map((workType) => workType.work_type_name);

  const mutation = useMutation({
    mutationFn: submitWork,
    onSuccess: () => {
      setDraft(EMPTY_DRAFT);
      setShowValidation(false);
    },
  });

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
    onRequestNameChange: (value: string) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        requestName: value,
      }));
    },
    onRequestTextChange: (value: string) => {
      if (mutation.isError || mutation.isSuccess) {
        mutation.reset();
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        requestText: value,
      }));
    },
    onSubmit: () => {
      setShowValidation(true);
      mutation.reset();

      const nextValidationErrors = validateDraft(draft, messages);
      if (hasValidationErrors(nextValidationErrors)) {
        return;
      }

      mutation.mutate({
        ...(draft.requestName.trim().length > 0 ? { name: draft.requestName } : {}),
        payload: draft.requestText,
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

  if (draft.workTypeName.length === 0 && draft.requestText.length === 0) {
    return {
      kind: "guidance",
      message: messages.statusMessages.emptyGuidance,
    };
  }

  if (draft.workTypeName.length === 0) {
    return {
      kind: "guidance",
      message: messages.statusMessages.workTypeOnly,
    };
  }

  if (draft.requestText.trim().length === 0) {
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
  if (validationErrors.workTypeName && validationErrors.requestText) {
    return messages.validationMessages.bothMissing;
  }
  if (validationErrors.workTypeName) {
    return validationErrors.workTypeName;
  }
  return validationErrors.requestText ?? messages.validationMessages.fallback;
}

function hasValidationErrors(validationErrors: SubmitWorkValidationErrors): boolean {
  return Boolean(validationErrors.requestText || validationErrors.workTypeName);
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
    validationErrors.workTypeName = messages.validationMessages.workTypeRequired;
  }

  if (draft.requestText.trim().length === 0) {
    validationErrors.requestText = messages.validationMessages.requestRequired;
  }

  return validationErrors;
}
