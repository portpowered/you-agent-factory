import {
  Button,
  DashboardWidgetFrame,
  Input,
  Select,
  Textarea,
} from "../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { cx } from "../../lib/cx";

export interface SubmitWorkDraft {
  requestName: string;
  requestText: string;
  workTypeName: string;
}

export interface SubmitWorkValidationErrors {
  workTypeName?: string;
}

export interface SubmitWorkStatus {
  kind: "error" | "guidance" | "submitting" | "success" | "validation-error";
  message: string;
}

export interface SubmitWorkCardProps {
  draft: SubmitWorkDraft;
  isSubmitting?: boolean;
  onRequestNameChange: (value: string) => void;
  onRequestTextChange: (value: string) => void;
  onSubmit: () => void;
  onWorkTypeNameChange: (value: string) => void;
  status: SubmitWorkStatus;
  submitWorkTypeNames: string[];
  validationErrors?: SubmitWorkValidationErrors;
  widgetId?: string;
}

const FORM_CLASS = "grid h-full min-h-0 gap-4";
const FIELD_GROUP_CLASS = "grid gap-2";
const FIELD_LABEL_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const ACTION_ROW_CLASS =
  "mt-auto grid gap-3 md:flex md:flex-wrap md:items-start md:justify-between";
const HELP_TEXT_CLASS = cx("max-w-xl leading-relaxed text-af-ink/66", DASHBOARD_SUPPORTING_TEXT_CLASS);
const VALIDATION_TEXT_CLASS = cx("text-af-danger-ink", DASHBOARD_SUPPORTING_TEXT_CLASS);
const STATUS_TONE_CLASS_BY_KIND: Record<SubmitWorkStatus["kind"], string> = {
  error: "text-af-danger-ink",
  guidance: "text-af-ink/66",
  submitting: "text-af-accent",
  success: "text-af-success-ink",
  "validation-error": "text-af-danger-ink",
};

export function SubmitWorkCard({
  draft,
  isSubmitting = false,
  onRequestNameChange,
  onRequestTextChange,
  onSubmit,
  onWorkTypeNameChange,
  status,
  submitWorkTypeNames,
  validationErrors,
  widgetId = "submit-work",
}: SubmitWorkCardProps) {
  const hasConfiguredWorkTypes = submitWorkTypeNames.length > 0;
  const hasSelectedWorkType = draft.workTypeName.length > 0;
  const controlsDisabled = !hasConfiguredWorkTypes || isSubmitting;
  const canSubmit =
    hasConfiguredWorkTypes && hasSelectedWorkType && !isSubmitting;
  const requestNameID = `${widgetId}-request-name`;
  const requestTextID = `${widgetId}-request-text`;
  const requestTextHintID = `${widgetId}-request-text-hint`;
  const workTypeID = `${widgetId}-work-type`;
  const workTypeErrorID = `${widgetId}-work-type-error`;
  const statusID = `${widgetId}-status`;

  return (
    <DashboardWidgetFrame title="Submit work" widgetId={widgetId}>
      <form
        className={FORM_CLASS}
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={workTypeID}>
            Work type
          </label>
          <Select
            aria-describedby={
              validationErrors?.workTypeName ? workTypeErrorID : undefined
            }
            aria-invalid={validationErrors?.workTypeName ? "true" : undefined}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            id={workTypeID}
            onChange={(event) => onWorkTypeNameChange(event.target.value)}
            value={draft.workTypeName}
          >
            <option value="">Select a work type</option>
            {submitWorkTypeNames.map((workTypeName) => (
              <option key={workTypeName} value={workTypeName}>
                {workTypeName}
              </option>
            ))}
          </Select>
          {validationErrors?.workTypeName ? (
            <p className={VALIDATION_TEXT_CLASS} id={workTypeErrorID}>
              {validationErrors.workTypeName}
            </p>
          ) : null}
        </div>

        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={requestNameID}>
            Request name
          </label>
          <Input
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            id={requestNameID}
            onChange={(event) => onRequestNameChange(event.target.value)}
            placeholder="Add an optional label for this request."
            type="text"
            value={draft.requestName}
          />
        </div>

        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={requestTextID}>
            Request
          </label>
          <Textarea
            aria-describedby={requestTextHintID}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            id={requestTextID}
            onChange={(event) => onRequestTextChange(event.target.value)}
            placeholder="Optional: describe what you want this request to accomplish."
            value={draft.requestText}
          />
          <p className={HELP_TEXT_CLASS} id={requestTextHintID}>
            Optional. Leave this blank to submit an empty request.
          </p>
        </div>

        <div className={ACTION_ROW_CLASS}>
          <p
            className={cx(
              HELP_TEXT_CLASS,
              STATUS_TONE_CLASS_BY_KIND[status.kind],
            )}
            id={statusID}
            role={
              status.kind === "error" || status.kind === "validation-error"
                ? "alert"
                : "status"
            }
          >
            {status.message}
          </p>
          <Button
            aria-busy={isSubmitting ? "true" : undefined}
            className="shrink-0"
            disabled={!canSubmit}
            tone={canSubmit ? "default" : "outline"}
            type="submit"
          >
            {isSubmitting ? "Submitting..." : "Submit work"}
          </Button>
        </div>
      </form>
    </DashboardWidgetFrame>
  );
}
