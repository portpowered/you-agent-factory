import { useMutation } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { stageSubmitWorkFile, submitWork } from "../../../api/work";
import type {
  SubmitWorkDraft,
  SubmitWorkDraftFileItem,
  SubmitWorkDraftItemType,
} from "../components/submit-work-card";
import type { SubmitWorkMessages } from "../messages/submit-work";
import {
  buildStatus,
  buildStructuredSubmitItems,
  createDefaultDraft,
  createDefaultTextItem,
  createDraftItem,
  fileToBase64,
  hasValidationErrors,
  normalizeMediaType,
  resetDraftPreservingWorkType,
  resetSubmitMutation,
  stageSubmitWorkErrorMessage,
  validateDraft,
} from "./use-submit-work-widget-helpers";

interface SubmitWorkSessionState {
  draft: SubmitWorkDraft;
  showValidation: boolean;
}

type SubmitWorkSubmissionState =
  | { kind: "error"; requestID: number; error: unknown }
  | { kind: "submitting"; requestID: number }
  | { kind: "success"; requestID: number; resultTraceID?: string };

function createSubmitWorkSessionState(): SubmitWorkSessionState {
  return {
    draft: createDefaultDraft(),
    showValidation: false,
  };
}

function sessionFileItemKey(sessionID: string, itemID: string): string {
  return `${sessionID}::${itemID}`;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this hook intentionally keeps the submit-work draft transitions visible in one place while the multimodal flow is still landing.
export function useSubmitWorkWidget(
  sessionID: string,
  submitWorkTypes: DashboardSubmitWorkType[],
  messages: SubmitWorkMessages,
) {
  const [sessionStates, setSessionStates] = useState<
    Record<string, SubmitWorkSessionState>
  >({});
  const [submissionStates, setSubmissionStates] = useState<
    Record<string, SubmitWorkSubmissionState>
  >({});
  const nextItemSequenceRef = useRef<Record<string, number>>({});
  const fileStageRequestIDsRef = useRef<Record<string, number>>({});
  const nextSubmissionRequestIDRef = useRef(0);
  const activeSubmissionRequestIDsRef = useRef<Record<string, number>>({});
  const currentSessionState =
    sessionStates[sessionID] ?? createSubmitWorkSessionState();
  const draft = currentSessionState.draft;
  const showValidation = currentSessionState.showValidation;
  const submitWorkTypeNames = submitWorkTypes.map(
    (workType) => workType.work_type_name,
  );

  const mutation = useMutation({
    mutationFn: (input: {
      request: Parameters<typeof submitWork>[0];
      sessionID: string;
    }) => submitWork(input.request, { sessionID: input.sessionID }),
  });
  const resetMutation = mutation.reset;

  useEffect(() => {
    if (sessionID.trim().length === 0) {
      return;
    }
    resetMutation();
  }, [resetMutation, sessionID]);

  useEffect(() => {
    if (
      draft.workTypeName.length > 0 &&
      !submitWorkTypeNames.includes(draft.workTypeName)
    ) {
      setSessionStates((currentStates) => {
        const currentState =
          currentStates[sessionID] ?? createSubmitWorkSessionState();
        return {
          ...currentStates,
          [sessionID]: {
            ...currentState,
            draft: {
              ...currentState.draft,
              workTypeName: "",
            },
          },
        };
      });
    }
  }, [draft.workTypeName, sessionID, submitWorkTypeNames]);

  const validationErrors = showValidation ? validateDraft(draft, messages) : {};

  const updateSessionState = (
    targetSessionID: string,
    update: (state: SubmitWorkSessionState) => SubmitWorkSessionState,
  ) => {
    setSessionStates((currentStates) => {
      const currentState =
        currentStates[targetSessionID] ?? createSubmitWorkSessionState();
      return {
        ...currentStates,
        [targetSessionID]: update(currentState),
      };
    });
  };

  const clearSubmissionState = (targetSessionID: string) => {
    activeSubmissionRequestIDsRef.current[targetSessionID] =
      nextSubmissionRequestIDRef.current + 1;
    setSubmissionStates((currentStates) => {
      if (!(targetSessionID in currentStates)) {
        return currentStates;
      }
      const nextStates = { ...currentStates };
      delete nextStates[targetSessionID];
      return nextStates;
    });
  };

  const stageFileItem = async (
    targetSessionID: string,
    itemId: string,
    itemType: SubmitWorkDraftFileItem["type"],
    file: File,
  ) => {
    const mediaType = normalizeMediaType(file);
    const fileKey = sessionFileItemKey(targetSessionID, itemId);
    const requestID = (fileStageRequestIDsRef.current[fileKey] ?? 0) + 1;
    fileStageRequestIDsRef.current[fileKey] = requestID;
    updateSessionState(targetSessionID, (currentState) => ({
      ...currentState,
      draft: {
        ...currentState.draft,
        items: currentState.draft.items.map((item) =>
          item.id === itemId && item.type !== "text"
            ? {
                ...item,
                fileName: file.name,
                mediaType,
                stagedFileRef: undefined,
                stagingError: undefined,
                stagingStatus: "staging",
                url: undefined,
              }
            : item,
        ),
      },
    }));

    try {
      const response = await stageSubmitWorkFile(
        {
          contentBase64: await fileToBase64(file),
          fileName: file.name,
          itemType,
          mediaType,
        },
        { sessionID: targetSessionID },
      );
      if (fileStageRequestIDsRef.current[fileKey] !== requestID) {
        return;
      }
      updateSessionState(targetSessionID, (currentState) => ({
        ...currentState,
        draft: {
          ...currentState.draft,
          items: currentState.draft.items.map((item) =>
            item.id === itemId && item.type !== "text"
              ? {
                  ...item,
                  fileName: response.fileName,
                  mediaType: response.mediaType,
                  stagedFileRef: response.stagedFileRef,
                  stagingError: undefined,
                  stagingStatus: "ready",
                  url: response.url,
                }
              : item,
          ),
        },
      }));
    } catch (error) {
      if (fileStageRequestIDsRef.current[fileKey] !== requestID) {
        return;
      }
      updateSessionState(targetSessionID, (currentState) => ({
        ...currentState,
        draft: {
          ...currentState.draft,
          items: currentState.draft.items.map((item) =>
            item.id === itemId && item.type !== "text"
              ? {
                  ...item,
                  fileName: file.name,
                  mediaType,
                  stagedFileRef: undefined,
                  stagingError: stageSubmitWorkErrorMessage(error, messages),
                  stagingStatus: "failure",
                  url: undefined,
                }
              : item,
          ),
        },
      }));
    }
  };

  const currentSubmission = submissionStates[sessionID];
  const isSubmitting = currentSubmission?.kind === "submitting";

  return {
    draft,
    isSubmitting,
    onAddItem: (type: SubmitWorkDraftItemType) => {
      resetSubmitMutation(mutation);
      clearSubmissionState(sessionID);
      const nextSequence = nextItemSequenceRef.current[sessionID] ?? 2;
      nextItemSequenceRef.current[sessionID] = nextSequence + 1;
      const nextItem = createDraftItem(type, nextSequence);
      updateSessionState(sessionID, (currentState) => ({
        ...currentState,
        draft: {
          ...currentState.draft,
          items: [...currentState.draft.items, nextItem],
        },
      }));
    },
    onItemTextChange: (itemId: string, value: string) => {
      resetSubmitMutation(mutation);
      clearSubmissionState(sessionID);
      updateSessionState(sessionID, (currentState) => ({
        ...currentState,
        draft: {
          ...currentState.draft,
          items: currentState.draft.items.map((item) =>
            item.id === itemId
              ? {
                  ...item,
                  text: value,
                }
              : item,
          ),
        },
      }));
    },
    onRemoveItem: (itemId: string) => {
      resetSubmitMutation(mutation);
      clearSubmissionState(sessionID);
      delete fileStageRequestIDsRef.current[
        sessionFileItemKey(sessionID, itemId)
      ];
      updateSessionState(sessionID, (currentState) => {
        const remainingItems = currentState.draft.items.filter(
          (item) => item.id !== itemId,
        );

        return {
          ...currentState,
          draft: {
            ...currentState.draft,
            items:
              remainingItems.length > 0
                ? remainingItems
                : [createDefaultTextItem()],
          },
        };
      });
    },
    onRequestNameChange: (value: string) => {
      resetSubmitMutation(mutation);
      clearSubmissionState(sessionID);
      updateSessionState(sessionID, (currentState) => ({
        ...currentState,
        draft: {
          ...currentState.draft,
          requestName: value,
        },
      }));
    },
    onStageFileItems: async (itemId: string, files: File[]) => {
      resetSubmitMutation(mutation);
      clearSubmissionState(sessionID);
      const targetItem = draft.items.find(
        (item): item is SubmitWorkDraftFileItem =>
          item.id === itemId && item.type !== "text",
      );
      if (!targetItem || files.length === 0) {
        return;
      }
      const [firstFile, ...additionalFiles] = files;
      const nextSequence = nextItemSequenceRef.current[sessionID] ?? 2;
      const additionalItems = additionalFiles.map((file, index) => ({
        file,
        item: createDraftItem(
          targetItem.type,
          nextSequence + index,
        ) as SubmitWorkDraftFileItem,
      }));
      nextItemSequenceRef.current[sessionID] =
        nextSequence + additionalItems.length;

      if (additionalItems.length > 0) {
        updateSessionState(sessionID, (currentState) => {
          const targetIndex = currentState.draft.items.findIndex(
            (item) => item.id === itemId,
          );
          if (targetIndex < 0) {
            return currentState;
          }

          return {
            ...currentState,
            draft: {
              ...currentState.draft,
              items: [
                ...currentState.draft.items.slice(0, targetIndex + 1),
                ...additionalItems.map(({ item }) => item),
                ...currentState.draft.items.slice(targetIndex + 1),
              ],
            },
          };
        });
      }

      void stageFileItem(sessionID, itemId, targetItem.type, firstFile);
      for (const additionalItem of additionalItems) {
        void stageFileItem(
          sessionID,
          additionalItem.item.id,
          additionalItem.item.type,
          additionalItem.file,
        );
      }
    },
    onSubmit: () => {
      clearSubmissionState(sessionID);
      setSessionStates((currentStates) => {
        const currentState =
          currentStates[sessionID] ?? createSubmitWorkSessionState();
        return {
          ...currentStates,
          [sessionID]: {
            ...currentState,
            showValidation: true,
          },
        };
      });
      mutation.reset();

      const nextValidationErrors = validateDraft(draft, messages);
      if (hasValidationErrors(nextValidationErrors)) {
        return;
      }

      const requestID = ++nextSubmissionRequestIDRef.current;
      activeSubmissionRequestIDsRef.current[sessionID] = requestID;
      setSubmissionStates((currentStates) => ({
        ...currentStates,
        [sessionID]: { kind: "submitting", requestID },
      }));

      void mutation
        .mutateAsync({
          request: {
            items: buildStructuredSubmitItems(draft),
            name: draft.requestName,
            workTypeName: draft.workTypeName,
          },
          sessionID,
        })
        .then((response) => {
          if (activeSubmissionRequestIDsRef.current[sessionID] !== requestID) {
            return;
          }

          const resultTraceID =
            response.traceId ?? (response as { trace_id?: string }).trace_id;
          setSubmissionStates((currentStates) => ({
            ...currentStates,
            [sessionID]: { kind: "success", requestID, resultTraceID },
          }));
          updateSessionState(sessionID, (currentState) => ({
            ...currentState,
            draft: resetDraftPreservingWorkType(
              currentState.draft.workTypeName,
            ),
            showValidation: false,
          }));
        })
        .catch((error: unknown) => {
          if (activeSubmissionRequestIDsRef.current[sessionID] !== requestID) {
            return;
          }
          setSubmissionStates((currentStates) => ({
            ...currentStates,
            [sessionID]: { kind: "error", requestID, error },
          }));
        });
    },
    onWorkTypeNameChange: (value: string) => {
      resetSubmitMutation(mutation);
      clearSubmissionState(sessionID);
      updateSessionState(sessionID, (currentState) => ({
        ...currentState,
        draft: {
          ...currentState.draft,
          workTypeName: value,
        },
      }));
    },
    status: buildStatus({
      draft,
      error:
        currentSubmission?.kind === "error"
          ? currentSubmission.error
          : undefined,
      isSubmitting,
      isSuccess: currentSubmission?.kind === "success",
      messages,
      resultTraceID:
        currentSubmission?.kind === "success"
          ? currentSubmission.resultTraceID
          : undefined,
      showValidation,
      submitWorkTypeNames,
    }),
    submitWorkTypeNames,
    validationErrors,
  };
}
