import { useMutation } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { stageSubmitWorkFile, submitWork } from "../../../api/work";
import type { SubmitWorkMessages } from "../messages/submit-work";
import type { SubmitWorkDraft, SubmitWorkDraftFileItem, SubmitWorkDraftItemType } from "../components/submit-work-card";
import {
  buildStatus,
  buildStructuredSubmitItems,
  createDefaultDraft,
  createDraftItem,
  createDefaultTextItem,
  fileToBase64,
  normalizeMediaType,
  resetDraftPreservingWorkType,
  resetSubmitMutation,
  stageSubmitWorkErrorMessage,
  validateDraft,
  hasValidationErrors,
} from "./use-submit-work-widget-helpers";

const EMPTY_DRAFT: SubmitWorkDraft = createDefaultDraft();

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this hook intentionally keeps the submit-work draft transitions visible in one place while the multimodal flow is still landing.
export function useSubmitWorkWidget(
  sessionID: string,
  submitWorkTypes: DashboardSubmitWorkType[],
  messages: SubmitWorkMessages,
) {
  const [draft, setDraft] = useState<SubmitWorkDraft>(EMPTY_DRAFT);
  const [showValidation, setShowValidation] = useState(false);
  const nextItemSequenceRef = useRef(2);
  const fileStageRequestIDsRef = useRef<Record<string, number>>({});
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
      fileStageRequestIDsRef.current = {};
      setShowValidation(false);
    },
  });
  const resetMutation = mutation.reset;

  useEffect(() => {
    if (sessionID.trim().length === 0) {
      return;
    }
    nextItemSequenceRef.current = 2;
    fileStageRequestIDsRef.current = {};
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

  const stageFileItem = async (
    itemId: string,
    itemType: SubmitWorkDraftFileItem["type"],
    file: File,
  ) => {
    const mediaType = normalizeMediaType(file);
    const requestID = (fileStageRequestIDsRef.current[itemId] ?? 0) + 1;
    fileStageRequestIDsRef.current[itemId] = requestID;
    setDraft((currentDraft) => ({
      ...currentDraft,
      items: currentDraft.items.map((item) =>
        item.id === itemId && item.type !== "text"
          ? {
              ...item,
              fileName: file.name,
              mediaType,
              stagedFileRef: undefined,
              stagingError: undefined,
              stagingStatus: "staging",
            }
          : item,
      ),
    }));

    try {
      const response = await stageSubmitWorkFile(
        {
          contentBase64: await fileToBase64(file),
          fileName: file.name,
          itemType,
          mediaType,
        },
        { sessionID },
      );
      if (fileStageRequestIDsRef.current[itemId] !== requestID) {
        return;
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        items: currentDraft.items.map((item) =>
          item.id === itemId && item.type !== "text"
            ? {
                ...item,
                fileName: response.fileName,
                mediaType: response.mediaType,
                stagedFileRef: response.stagedFileRef,
                stagingError: undefined,
                stagingStatus: "ready",
              }
            : item,
        ),
      }));
    } catch (error) {
      if (fileStageRequestIDsRef.current[itemId] !== requestID) {
        return;
      }
      setDraft((currentDraft) => ({
        ...currentDraft,
        items: currentDraft.items.map((item) =>
          item.id === itemId && item.type !== "text"
            ? {
                ...item,
                fileName: file.name,
                mediaType,
                stagedFileRef: undefined,
                stagingError: stageSubmitWorkErrorMessage(error, messages),
                stagingStatus: "failure",
              }
            : item,
        ),
      }));
    }
  };

  return {
    draft,
    isSubmitting: mutation.isPending,
    onAddItem: (type: SubmitWorkDraftItemType) => {
      resetSubmitMutation(mutation);
      const nextItem = createDraftItem(type, nextItemSequenceRef.current);
      nextItemSequenceRef.current += 1;
      setDraft((currentDraft) => ({
        ...currentDraft,
        items: [...currentDraft.items, nextItem],
      }));
    },
    onItemTextChange: (itemId: string, value: string) => {
      resetSubmitMutation(mutation);
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
    onRemoveItem: (itemId: string) => {
      resetSubmitMutation(mutation);
      delete fileStageRequestIDsRef.current[itemId];
      setDraft((currentDraft) => {
        const remainingItems = currentDraft.items.filter((item) => item.id !== itemId);

        return {
          ...currentDraft,
          items: remainingItems.length > 0 ? remainingItems : [createDefaultTextItem()],
        };
      });
    },
    onRequestNameChange: (value: string) => {
      resetSubmitMutation(mutation);
      setDraft((currentDraft) => ({
        ...currentDraft,
        requestName: value,
      }));
    },
    onStageFileItems: async (itemId: string, files: File[]) => {
      resetSubmitMutation(mutation);
      const targetItem = draft.items.find(
        (item): item is SubmitWorkDraftFileItem =>
          item.id === itemId && item.type !== "text",
      );
      if (!targetItem || files.length === 0) {
        return;
      }
      const [firstFile, ...additionalFiles] = files;
      const additionalItems = additionalFiles.map((file, index) => ({
        file,
        item: createDraftItem(
          targetItem.type,
          nextItemSequenceRef.current + index,
        ) as SubmitWorkDraftFileItem,
      }));
      nextItemSequenceRef.current += additionalItems.length;

      if (additionalItems.length > 0) {
        setDraft((currentDraft) => {
          const targetIndex = currentDraft.items.findIndex((item) => item.id === itemId);
          if (targetIndex < 0) {
            return currentDraft;
          }

          return {
            ...currentDraft,
            items: [
              ...currentDraft.items.slice(0, targetIndex + 1),
              ...additionalItems.map(({ item }) => item),
              ...currentDraft.items.slice(targetIndex + 1),
            ],
          };
        });
      }

      void stageFileItem(itemId, targetItem.type, firstFile);
      for (const additionalItem of additionalItems) {
        void stageFileItem(
          additionalItem.item.id,
          additionalItem.item.type,
          additionalItem.file,
        );
      }
    },
    onSubmit: () => {
      setShowValidation(true);
      mutation.reset();

      const nextValidationErrors = validateDraft(draft, messages);
      if (hasValidationErrors(nextValidationErrors)) {
        return;
      }

      mutation.mutate({
        items: buildStructuredSubmitItems(draft),
        name: draft.requestName,
        workTypeName: draft.workTypeName,
      });
    },
    onWorkTypeNameChange: (value: string) => {
      resetSubmitMutation(mutation);
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
