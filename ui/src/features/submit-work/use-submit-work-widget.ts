import { useMutation } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { DashboardSubmitWorkType } from "../../api/dashboard/types";
import { isSubmitWorkAPIError, submitWork } from "../../api/work";
import type {
  SubmitWorkDraft,
  SubmitWorkStatus,
  SubmitWorkValidationErrors,
} from "./submit-work-card";

const EMPTY_DRAFT: SubmitWorkDraft = {
  requestName: "",
  requestText: "",
  workTypeName: "",
};

export function useSubmitWorkWidget(
  submitWorkTypes: DashboardSubmitWorkType[],
) {
  const [draft, setDraft] = useState<SubmitWorkDraft>(EMPTY_DRAFT);
  const [showValidation, setShowValidation] = useState(false);
  const submitWorkTypeNames = submitWorkTypes.map(
    (workType) => workType.work_type_name,
  );

  const mutation = useMutation({
    mutationFn: submitWork,
    onSuccess: () => {
      setDraft((currentDraft) => ({
        ...EMPTY_DRAFT,
        workTypeName: currentDraft.workTypeName,
      }));
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

  const validationErrors = showValidation ? validateDraft(draft) : {};

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

      const nextValidationErrors = validateDraft(draft);
      if (hasValidationErrors(nextValidationErrors)) {
        return;
      }

      mutation.mutate({
        ...(draft.requestName.trim().length > 0
          ? { name: draft.requestName }
          : {}),
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
      resultTraceID: mutation.data?.traceId,
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
  resultTraceID,
  showValidation,
  submitWorkTypeNames,
}: {
  draft: SubmitWorkDraft;
  error: unknown;
  isSubmitting: boolean;
  isSuccess: boolean;
  resultTraceID?: string;
  showValidation: boolean;
  submitWorkTypeNames: string[];
}): SubmitWorkStatus {
  if (isSubmitting) {
    return {
      kind: "submitting",
      message: "Sending your request...",
    };
  }

  if (error) {
    return {
      kind: "error",
      message: submitWorkErrorMessage(error),
    };
  }

  if (isSuccess) {
    return {
      kind: "success",
      message: `Your request was submitted. Trace ID: ${resultTraceID ?? "unavailable"}.`,
    };
  }

  if (submitWorkTypeNames.length === 0) {
    return {
      kind: "guidance",
      message: "No work types are available to submit right now.",
    };
  }

  const validationErrors = validateDraft(draft);
  if (showValidation && hasValidationErrors(validationErrors)) {
    return {
      kind: "validation-error",
      message: buildValidationSummary(validationErrors),
    };
  }

  if (draft.workTypeName.length === 0) {
    return {
      kind: "guidance",
      message: "Choose a work type to continue. Request details are optional.",
    };
  }

  return {
    kind: "guidance",
    message: "Ready to submit. Request details are optional.",
  };
}

function buildValidationSummary(
  validationErrors: SubmitWorkValidationErrors,
): string {
  if (validationErrors.workTypeName) {
    return validationErrors.workTypeName;
  }
  return "Fix the highlighted fields before submitting.";
}

function hasValidationErrors(
  validationErrors: SubmitWorkValidationErrors,
): boolean {
  return Boolean(validationErrors.workTypeName);
}

function submitWorkErrorMessage(error: unknown): string {
  if (isSubmitWorkAPIError(error) && error.message.length > 0) {
    return error.message;
  }
  return "We couldn't submit your request. Try again in a moment.";
}

function validateDraft(draft: SubmitWorkDraft): SubmitWorkValidationErrors {
  const validationErrors: SubmitWorkValidationErrors = {};

  if (draft.workTypeName.length === 0) {
    validationErrors.workTypeName = "Choose a work type before submitting.";
  }

  return validationErrors;
}
