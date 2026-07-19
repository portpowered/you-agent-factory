import { useState } from "react";

import { Button, Label, Textarea } from "../../../components/ui";
import {
  type FactorySimpleSubmissionAvailability,
  type FactorySimpleSubmissionEligibilityInput,
  resolveFactorySimpleSubmissionAvailability,
} from "../lib/factory-simple-submission-eligibility";

type FactorySimpleSubmissionUnavailableReason = Extract<
  FactorySimpleSubmissionAvailability,
  { kind: "unavailable" }
>["reason"];

export interface FactorySimpleSubmissionComposerProps
  extends FactorySimpleSubmissionEligibilityInput {
  draft: string;
  isSubmitting?: boolean;
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

const DEFAULT_UNAVAILABLE_MESSAGES = {
  "ambiguous-default":
    "Multiple default work types are configured, so a submission cannot be routed safely.",
  closed: "This Factory is closed and cannot accept submissions.",
  error: "This Factory has an error and cannot accept submissions.",
  history: "Return to the latest Factory state to submit work.",
  invalid: "This Factory is invalid and cannot accept submissions.",
  loading: "The Factory is still loading. Try again when it is ready.",
  "no-default":
    "No eligible default work type is available for text submissions.",
} as const;

function resizeTextarea(textarea: HTMLTextAreaElement) {
  textarea.style.height = "auto";
  textarea.style.height = `${textarea.scrollHeight}px`;
}

export function FactorySimpleSubmissionComposer({
  draft,
  factoryState,
  isCurrent,
  isSubmitting = false,
  onDraftChange,
  onSubmit,
  submissionError,
  unavailableMessage = (reason) => DEFAULT_UNAVAILABLE_MESSAGES[reason],
  workTypes,
}: FactorySimpleSubmissionComposerProps) {
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
        error instanceof Error
          ? error.message
          : "We couldn't submit this work. Try again.",
      );
    } finally {
      setIsLocallySubmitting(false);
    }
  };

  return (
    <form
      aria-label="Simple work submission"
      className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="grid gap-1">
        <label htmlFor="factory-simple-submission-draft">
          <Label>Submit text</Label>
        </label>
        <Textarea
          aria-describedby={
            unavailableReason ? "factory-simple-submission-status" : undefined
          }
          disabled={isDisabled}
          id="factory-simple-submission-draft"
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
          placeholder="Describe the work to submit."
          className="min-h-24 max-h-48 resize-none overflow-y-auto"
          value={draft}
        />
      </div>
      <Button disabled={isDisabled || isDraftBlank} type="submit">
        {isSubmitPending ? "Submitting..." : "Submit"}
      </Button>
      {unavailableReason ? (
        <p
          className="sm:col-span-2"
          id="factory-simple-submission-status"
          role="status"
        >
          {unavailableMessage(unavailableReason)}
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
