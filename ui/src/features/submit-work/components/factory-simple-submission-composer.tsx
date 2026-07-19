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
  onSubmit: (workTypeName: string) => void;
  unavailableMessage?: (
    reason: FactorySimpleSubmissionUnavailableReason,
  ) => string;
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

export function FactorySimpleSubmissionComposer({
  draft,
  factoryState,
  isCurrent,
  isSubmitting = false,
  onDraftChange,
  onSubmit,
  unavailableMessage = (reason) => DEFAULT_UNAVAILABLE_MESSAGES[reason],
  workTypes,
}: FactorySimpleSubmissionComposerProps) {
  const availability = resolveFactorySimpleSubmissionAvailability({
    factoryState,
    isCurrent,
    workTypes,
  });
  const unavailableReason =
    availability.kind === "unavailable" ? availability.reason : undefined;
  const isAvailable = availability.kind === "available";
  const isDraftBlank = draft.trim().length === 0;
  const isDisabled = !isAvailable || isSubmitting;

  return (
    <form
      aria-label="Simple work submission"
      className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
      onSubmit={(event) => {
        event.preventDefault();
        if (
          availability.kind === "available" &&
          !isDraftBlank &&
          !isSubmitting
        ) {
          onSubmit(availability.workTypeName);
        }
      }}
    >
      <div className="grid gap-1">
        <Label htmlFor="factory-simple-submission-draft">Submit text</Label>
        <Textarea
          aria-describedby={
            unavailableReason ? "factory-simple-submission-status" : undefined
          }
          disabled={isDisabled}
          id="factory-simple-submission-draft"
          onChange={(event) => onDraftChange(event.target.value)}
          placeholder="Describe the work to submit."
          value={draft}
        />
      </div>
      <Button disabled={isDisabled || isDraftBlank} type="submit">
        {isSubmitting ? "Submitting..." : "Submit"}
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
    </form>
  );
}
