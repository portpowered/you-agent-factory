import { Label } from "@you-agent-factory/components/primitives";
import { useEffect, useId, useRef, useState } from "react";
import { Button } from "../../../../components/ui/button";
import { Textarea } from "../../../../components/ui/textarea";
import {
  type FactorySimpleSubmissionAvailability,
  type FactorySimpleSubmissionEligibilityInput,
  resolveFactorySimpleSubmissionAvailability,
} from "../../lib/factory-simple-submission-eligibility";
import { getSubmitWorkMessages } from "../../messages/submit-work";

type FactorySimpleSubmissionUnavailableReason = Extract<
  FactorySimpleSubmissionAvailability,
  { kind: "unavailable" }
>["reason"];

export interface FactorySimpleSubmissionComposerProps
  extends FactorySimpleSubmissionEligibilityInput {
  draft: string;
  isSubmitting?: boolean;
  locale?: string;
  onDraftChange: (value: string) => void;
  onSubmit: (submission: FactorySimpleTextSubmission) => Promise<void>;
  submissionError?: string;
  submissionSuccess?: string;
  sessionID?: string;
  unavailableMessage?: (
    reason: FactorySimpleSubmissionUnavailableReason,
  ) => string;
}

/** The text-only submission shape supplied to a host-owned transport adapter. */
export interface FactorySimpleTextSubmission {
  content: readonly [{ text: string; type: "text" }];
  workTypeName: string;
}

function resizeTextarea(textarea: HTMLTextAreaElement) {
  textarea.style.height = "auto";
  textarea.style.height = `${textarea.scrollHeight}px`;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the simple composer keeps its session-switch and focus guards next to the form submission lifecycle.
export function FactorySimpleSubmissionComposer({
  draft,
  factoryState,
  isCurrent,
  isSubmitting = false,
  locale,
  onDraftChange,
  onSubmit,
  submissionError,
  submissionSuccess,
  sessionID,
  unavailableMessage,
  workTypes,
}: FactorySimpleSubmissionComposerProps) {
  const submitWorkMessages = getSubmitWorkMessages(locale);
  const messages = submitWorkMessages.simpleComposer;
  const instanceID = useId().replaceAll(":", "");
  const draftID = `factory-simple-submission-draft-${instanceID}`;
  const statusID = `factory-simple-submission-status-${instanceID}`;
  const errorID = `factory-simple-submission-error-${instanceID}`;
  const successID = `factory-simple-submission-success-${instanceID}`;
  const [localSubmissionError, setLocalSubmissionError] = useState<string>();
  const [isLocallySubmitting, setIsLocallySubmitting] = useState(false);
  const currentSessionIDRef = useRef(sessionID);
  const restoreFocusAfterSubmission = useRef(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const availability = resolveFactorySimpleSubmissionAvailability({
    factoryState,
    isCurrent,
    workTypes,
  });
  const unavailableReason =
    availability.kind === "unavailable" ? availability.reason : undefined;
  const isAvailable = availability.kind === "available";
  const isDraftBlank = draft.trim().length === 0;
  const isSubmitPending = isSubmitting || isLocallySubmitting;
  const isDisabled = !isAvailable || isSubmitPending;
  const errorMessage = submissionError ?? localSubmissionError;
  const describedBy = [
    unavailableReason ? statusID : undefined,
    errorMessage ? errorID : undefined,
    submissionSuccess ? successID : undefined,
  ]
    .filter((id): id is string => id !== undefined)
    .join(" ");

  useEffect(() => {
    currentSessionIDRef.current = sessionID;
    if (sessionID?.trim().length === 0) {
      return;
    }
    setLocalSubmissionError(undefined);
    setIsLocallySubmitting(false);
    restoreFocusAfterSubmission.current = false;
  }, [sessionID]);

  useEffect(() => {
    if (isSubmitPending || !restoreFocusAfterSubmission.current) return;
    restoreFocusAfterSubmission.current = false;
    textareaRef.current?.focus();
  }, [isSubmitPending]);

  const submit = async () => {
    if (availability.kind !== "available" || isDraftBlank || isSubmitPending) {
      return;
    }

    setLocalSubmissionError(undefined);
    setIsLocallySubmitting(true);
    restoreFocusAfterSubmission.current = true;
    const submissionSessionID = sessionID;
    try {
      await onSubmit({
        content: [{ text: draft, type: "text" }],
        workTypeName: availability.workTypeName,
      });
      onDraftChange("");
    } catch (error) {
      if (currentSessionIDRef.current !== submissionSessionID) {
        return;
      }
      setLocalSubmissionError(
        error instanceof Error ? error.message : messages.errorFallback,
      );
    } finally {
      if (currentSessionIDRef.current === submissionSessionID) {
        setIsLocallySubmitting(false);
      }
    }
  };

  return (
    <form
      aria-label={messages.formLabel}
      aria-describedby={describedBy || undefined}
      className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {sessionID ? (
        <p
          className="min-w-0 break-words sm:col-span-2"
          data-submit-work-destination=""
        >
          <span className="text-on-surface-variant">
            {submitWorkMessages.destinationLabel}:{" "}
          </span>
          <span className="break-words">{sessionID}</span>
        </p>
      ) : null}
      <div className="grid gap-1">
        <label htmlFor={draftID}>
          <Label>{messages.textLabel}</Label>
        </label>
        <Textarea
          aria-describedby={describedBy || undefined}
          disabled={isDisabled}
          id={draftID}
          onChange={(event) => {
            onDraftChange(event.target.value);
            resizeTextarea(event.target);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              void submit();
            }
          }}
          placeholder={messages.placeholder}
          ref={textareaRef}
          className="min-h-24 max-h-48 resize-none overflow-y-auto"
          value={draft}
        />
      </div>
      <Button disabled={isDisabled || isDraftBlank} type="submit">
        {isSubmitPending ? messages.submittingAction : messages.submitAction}
      </Button>
      {unavailableReason ? (
        <p className="sm:col-span-2" id={statusID} role="status">
          {unavailableMessage?.(unavailableReason) ??
            messages.unavailable[unavailableReason]}
        </p>
      ) : null}
      {submissionSuccess ? (
        <p
          className="min-w-0 break-words sm:col-span-2"
          id={successID}
          role="status"
        >
          {submissionSuccess}
        </p>
      ) : null}
      {errorMessage ? (
        <p
          className="min-w-0 break-words sm:col-span-2"
          id={errorID}
          role="alert"
        >
          {errorMessage}
        </p>
      ) : null}
    </form>
  );
}
