import { useMutation } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { isSubmitWorkAPIError, submitWork } from "../../../api/work";
import type { SubmitWorkMessages } from "../messages/submit-work";
import type {
  SubmitWorkDraft,
  SubmitWorkStatus,
  SubmitWorkValidationErrors,
} from "../components/submit-work-card";

const EMPTY_DRAFT: SubmitWorkDraft = {
  requestName: "",
  requestText: "",
  workTypeName: "",
};

export function useSubmitWorkWidget(
  sessionID: string,
  submitWorkTypes: DashboardSubmitWorkType[],
  messages: SubmitWorkMessages,
) {
  const [draft, setDraft] = useState<SubmitWorkDraft>(EMPTY_DRAFT);
  const [showValidation, setShowValidation] = useState(false);
  const submitWorkTypeNames = submitWorkTypes.map(
    (workType) => workType.work_type_name,
  );

  const mutation = useMutation({
    mutationFn: (request: Parameters<typeof submitWork>[0]) =>
      submitWork(request, { sessionID }),
    onSuccess: () => {
      setDraft((currentDraft) => ({
        ...EMPTY_DRAFT,
        workTypeName: currentDraft.workTypeName,
      }));
      setShowValidation(false);
    },
  });
  const resetMutation = mutation.reset;

  useEffect(() => {
    if (sessionID.trim().length === 0) {
      return;
    }
    setDraft(EMPTY_DRAFT);
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
        name: draft.requestName,
        payload: draft.requestText.trim().length === 0 ? "" : draft.requestText,
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
