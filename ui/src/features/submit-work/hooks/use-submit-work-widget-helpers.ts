import {
  isStageSubmitWorkFileAPIError,
  isSubmitWorkAPIError,
  type submitWork,
} from "../../../api/work";
import type { SubmitWorkMessages } from "../messages/submit-work";
import type {
  SubmitWorkDraft,
  SubmitWorkDraftItem,
  SubmitWorkDraftItemType,
  SubmitWorkStatus,
  SubmitWorkValidationErrors,
} from "../components/submit-work-card";

export const DEFAULT_TEXT_ITEM_ID = "submission-item-1";
export type StructuredSubmitItems = NonNullable<Parameters<typeof submitWork>[0]["items"]>;

export function createDefaultDraft(): SubmitWorkDraft {
  return {
    items: [createDefaultTextItem()],
    requestName: "",
    workTypeName: "",
  };
}

export function createDefaultTextItem(): SubmitWorkDraftItem {
  return {
    id: DEFAULT_TEXT_ITEM_ID,
    text: "",
    type: "text",
  };
}

export function createDraftItem(
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
    stagingStatus: "idle",
    type,
  };
}

export function resetDraftPreservingWorkType(workTypeName: string): SubmitWorkDraft {
  return {
    ...createDefaultDraft(),
    workTypeName,
  };
}

export function buildStatus({
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

  if (hasIncompleteFileItems(draft)) {
    return {
      kind: "guidance",
      message: messages.statusMessages.fileItemsNeedAttention,
    };
  }

  return {
    kind: "guidance",
    message: messages.statusMessages.ready,
  };
}

export function buildValidationSummary(
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
  if (validationErrors.submissionItems) {
    return validationErrors.submissionItems;
  }
  return messages.validationMessages.fallback;
}

export function hasValidationErrors(
  validationErrors: SubmitWorkValidationErrors,
): boolean {
  return Boolean(
    validationErrors.requestName ||
      validationErrors.submissionItems ||
      validationErrors.workTypeName,
  );
}

export function submitWorkErrorMessage(
  error: unknown,
  messages: SubmitWorkMessages,
): string {
  if (isSubmitWorkAPIError(error) && error.message.length > 0) {
    return error.message;
  }
  return messages.statusMessages.errorFallback;
}

export function stageSubmitWorkErrorMessage(
  error: unknown,
  messages: SubmitWorkMessages,
): string {
  if (isStageSubmitWorkFileAPIError(error) && error.message.length > 0) {
    return error.message;
  }
  return messages.statusMessages.errorFallback;
}

export function validateDraft(
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
  if (!hasMeaningfulSubmissionItems(draft)) {
    validationErrors.submissionItems =
      messages.validationMessages.submissionItemsRequired;
    return validationErrors;
  }
  if (hasStagingFileItems(draft)) {
    validationErrors.submissionItems =
      messages.validationMessages.fileItemStillStaging;
    return validationErrors;
  }
  if (hasIncompleteFileItems(draft)) {
    validationErrors.submissionItems =
      messages.validationMessages.fileItemNeedsStaging;
  }

  return validationErrors;
}

export function buildStructuredSubmitItems(draft: SubmitWorkDraft) {
  const items: StructuredSubmitItems = [];

  for (const item of draft.items) {
    if (item.type === "text") {
      if (item.text.trim().length > 0) {
        items.push({
          text: item.text,
          type: "text",
        });
      }
      continue;
    }

    if (!item.stagedFileRef || !item.fileName || !item.mediaType) {
      continue;
    }

    items.push({
      fileName: item.fileName,
      mediaType: item.mediaType,
      stagedFileRef: item.stagedFileRef,
      type: item.type,
    });
  }

  return items;
}

export function normalizeMediaType(file: File): string {
  return file.type.trim().length > 0 ? file.type : "application/octet-stream";
}

export function resetSubmitMutation(mutation: {
  isError: boolean;
  isSuccess: boolean;
  reset: () => void;
}) {
  if (mutation.isError || mutation.isSuccess) {
    mutation.reset();
  }
}

export async function fileToBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  for (const value of bytes) {
    binary += String.fromCharCode(value);
  }
  return btoa(binary);
}

function hasMeaningfulSubmissionItems(draft: SubmitWorkDraft): boolean {
  return draft.items.some((item) => {
    if (item.type === "text") {
      return item.text.trim().length > 0;
    }
    return item.stagingStatus === "ready" && (item.stagedFileRef ?? "").length > 0;
  });
}

function hasIncompleteFileItems(draft: SubmitWorkDraft): boolean {
  return draft.items.some(
    (item) =>
      item.type !== "text" &&
      item.stagingStatus !== "ready",
  );
}

function hasStagingFileItems(draft: SubmitWorkDraft): boolean {
  return draft.items.some(
    (item) => item.type !== "text" && item.stagingStatus === "staging",
  );
}
