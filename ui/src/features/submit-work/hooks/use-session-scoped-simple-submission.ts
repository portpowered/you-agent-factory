import { useMutation } from "@tanstack/react-query";
import { useRef, useState } from "react";

import { submitWork } from "../../../api/work";
import type { FactorySimpleTextSubmission } from "../components/composer/factory-simple-submission-composer";
import type { SubmitWorkMessages } from "../messages/submit-work";

type SimpleSubmissionState =
  | { kind: "error"; requestID: number; error: unknown }
  | { kind: "submitting"; requestID: number }
  | { kind: "success"; requestID: number; resultTraceID?: string };

function removeSimpleSubmissionState(
  states: Record<string, SimpleSubmissionState>,
  sessionID: string,
): Record<string, SimpleSubmissionState> {
  if (!(sessionID in states)) {
    return states;
  }
  const nextStates = { ...states };
  delete nextStates[sessionID];
  return nextStates;
}

export function useSessionScopedSimpleSubmission(
  sessionID: string,
  messages: SubmitWorkMessages,
) {
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [submissionStates, setSubmissionStates] = useState<
    Record<string, SimpleSubmissionState>
  >({});
  const nextRequestIDRef = useRef(0);
  const activeRequestIDsRef = useRef<Record<string, number>>({});
  const mutation = useMutation({
    mutationFn: (input: {
      sessionID: string;
      submission: FactorySimpleTextSubmission;
    }) =>
      submitWork(
        {
          content: [...input.submission.content],
          workTypeName: input.submission.workTypeName,
        },
        { sessionID: input.sessionID },
      ),
  });
  const draft = drafts[sessionID] ?? "";
  const submissionState = submissionStates[sessionID];

  const onDraftChange = (value: string) => {
    mutation.reset();
    setDrafts((currentDrafts) => ({ ...currentDrafts, [sessionID]: value }));
    setSubmissionStates((currentStates) => {
      if (value.length === 0 && currentStates[sessionID]?.kind === "success") {
        return currentStates;
      }
      return removeSimpleSubmissionState(currentStates, sessionID);
    });
    activeRequestIDsRef.current[sessionID] = nextRequestIDRef.current + 1;
  };

  const onSubmit = async (submission: FactorySimpleTextSubmission) => {
    const requestID = ++nextRequestIDRef.current;
    activeRequestIDsRef.current[sessionID] = requestID;
    setSubmissionStates((currentStates) => ({
      ...currentStates,
      [sessionID]: { kind: "submitting", requestID },
    }));
    try {
      const response = await mutation.mutateAsync({ sessionID, submission });
      if (activeRequestIDsRef.current[sessionID] !== requestID) {
        return;
      }
      setSubmissionStates((currentStates) => ({
        ...currentStates,
        [sessionID]: {
          kind: "success",
          requestID,
          resultTraceID:
            response.traceId ?? (response as { trace_id?: string }).trace_id,
        },
      }));
    } catch (error) {
      if (activeRequestIDsRef.current[sessionID] === requestID) {
        setSubmissionStates((currentStates) => ({
          ...currentStates,
          [sessionID]: { kind: "error", requestID, error },
        }));
      }
      throw error;
    }
  };

  return {
    draft,
    isSubmitting: submissionState?.kind === "submitting",
    onDraftChange,
    onSubmit,
    submissionError:
      submissionState?.kind === "error"
        ? submissionState.error instanceof Error
          ? submissionState.error.message
          : messages.simpleComposer.errorFallback
        : undefined,
    submissionSuccess:
      submissionState?.kind === "success"
        ? messages.statusMessages.success(
            submissionState.resultTraceID ?? "unavailable",
          )
        : undefined,
  };
}
