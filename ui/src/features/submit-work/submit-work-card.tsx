import { cx } from "../../components/dashboard/classnames";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import { Button, DashboardWidgetFrame, Input, Select, Textarea } from "../../components/ui";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";

export interface SubmitWorkDraft {
  requestName: string;
  requestText: string;
  workTypeName: string;
}

export interface SubmitWorkValidationErrors {
  requestText?: string;
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
  "mt-auto flex flex-wrap items-start justify-between gap-3 max-[720px]:grid";
const HELP_TEXT_CLASS = cx("max-w-[32rem] leading-relaxed text-af-ink/66", DASHBOARD_SUPPORTING_TEXT_CLASS);
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
  const hasRequestText = draft.requestText.trim().length > 0;
  const controlsDisabled = !hasConfiguredWorkTypes || isSubmitting;
  const canSubmit = hasConfiguredWorkTypes && hasSelectedWorkType && hasRequestText && !isSubmitting;
  const workTypeErrorID = `${widgetId}-work-type-error`;
  const requestTextErrorID = `${widgetId}-request-text-error`;
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
          <p className={WIDGET_SUBTITLE_CLASS}>
            Send a new request to the current factory from the dashboard.
          </p>
        </div>

        <label className={FIELD_GROUP_CLASS}>
          <span className={FIELD_LABEL_CLASS}>Work type</span>
          <Select
            aria-label="Work type"
            aria-describedby={validationErrors?.workTypeName ? workTypeErrorID : undefined}
            aria-invalid={validationErrors?.workTypeName ? "true" : undefined}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
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
        </label>

        <label className={FIELD_GROUP_CLASS}>
          <span className={FIELD_LABEL_CLASS}>Request name</span>
          <Input
            aria-label="Request name"
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            onChange={(event) => onRequestNameChange(event.target.value)}
            placeholder="Add an optional label for this request."
            type="text"
            value={draft.requestName}
          />
        </label>

        <label className={FIELD_GROUP_CLASS}>
          <span className={FIELD_LABEL_CLASS}>Request</span>
          <Textarea
            aria-label="Request text"
            aria-describedby={validationErrors?.requestText ? requestTextErrorID : undefined}
            aria-invalid={validationErrors?.requestText ? "true" : undefined}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            onChange={(event) => onRequestTextChange(event.target.value)}
            placeholder="Describe what you want this request to accomplish."
            value={draft.requestText}
          />
          {validationErrors?.requestText ? (
            <p className={VALIDATION_TEXT_CLASS} id={requestTextErrorID}>
              {validationErrors.requestText}
            </p>
          ) : null}
        </label>

        <div className={ACTION_ROW_CLASS}>
          <p
            className={cx(HELP_TEXT_CLASS, STATUS_TONE_CLASS_BY_KIND[status.kind])}
            id={statusID}
            role={status.kind === "error" || status.kind === "validation-error" ? "alert" : "status"}
          >
            {status.message}
          </p>
          <Button
            aria-busy={isSubmitting ? "true" : undefined}
            className="shrink-0"
            disabled={!canSubmit}
            type="submit"
          >
            {isSubmitting ? "Submitting..." : "Submit work"}
          </Button>
        </div>
      </form>
    </DashboardWidgetFrame>
  );
}
