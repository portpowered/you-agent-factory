import { useId, useState } from "react";

import { Button, Label, Textarea } from "../../../../components/ui";
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

export function FactorySimpleSubmissionComposer({
  draft,
  factoryState,
  isCurrent,
  isSubmitting = false,
  locale,
  onDraftChange,
  onSubmit,
  submissionError,
  unavailableMessage,
  workTypes,
}: FactorySimpleSubmissionComposerProps) {
  const messages = getSubmitWorkMessages(locale).simpleComposer;
  const instanceID = useId().replaceAll(":", "");
  const draftID = `factory-simple-submission-draft-${instanceID}`;
  const statusID = `factory-simple-submission-status-${instanceID}`;
  const [localSubmissionError, setLocalSubmissionError] = useState<string>();
  const [isLocallySubmitting, setIsLocallySubmitting] = useState(false);
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

  const submit = async () => {
    if (availability.kind !== "available" || isDraftBlank || isSubmitPending) {
      return;
    }

    setLocalSubmissionError(undefined);
    setIsLocallySubmitting(true);
    try {
      await onSubmit({
        content: [{ text: draft, type: "text" }],
        workTypeName: availability.workTypeName,
      });
      onDraftChange("");
    } catch (error) {
      setLocalSubmissionError(
        error instanceof Error ? error.message : messages.errorFallback,
      );
    } finally {
      setIsLocallySubmitting(false);
    }
  };

  return (
    <form
      aria-label={messages.formLabel}
      className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="grid gap-1">
        <label htmlFor={draftID}>
          <Label>{messages.textLabel}</Label>
        </label>
        <Textarea
          aria-describedby={unavailableReason ? statusID : undefined}
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
      {errorMessage ? (
        <p className="sm:col-span-2" role="alert">
          {errorMessage}
        </p>
      ) : null}
    </form>
  );
}
