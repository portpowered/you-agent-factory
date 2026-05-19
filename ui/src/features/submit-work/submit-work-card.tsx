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
import { getSubmitWorkMessages } from "./messages/submit-work";

export interface SubmitWorkDraft {
  requestName: string;
  requestText: string;
  workTypeName: string;
}

export interface SubmitWorkValidationErrors {
  requestName?: string;
  workTypeName?: string;
}

export interface SubmitWorkStatus {
  kind: "error" | "guidance" | "submitting" | "success" | "validation-error";
  message: string;
}

export interface SubmitWorkCardProps {
  draft: SubmitWorkDraft;
  isSubmitting?: boolean;
  locale?: string;
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
  locale,
  onRequestNameChange,
  onRequestTextChange,
  onSubmit,
  onWorkTypeNameChange,
  status,
  submitWorkTypeNames,
  validationErrors,
  widgetId = "submit-work",
}: SubmitWorkCardProps) {
  const messages = getSubmitWorkMessages(locale);
  const hasConfiguredWorkTypes = submitWorkTypeNames.length > 0;
  const hasSelectedWorkType = draft.workTypeName.length > 0;
  const hasValidRequestName = draft.requestName.trim().length > 0;
  const controlsDisabled = !hasConfiguredWorkTypes || isSubmitting;
  const canSubmit =
    hasConfiguredWorkTypes &&
    hasSelectedWorkType &&
    hasValidRequestName &&
    !isSubmitting;
  const requestHint = messages.requestHint?.trim();
  const requestNameID = `${widgetId}-request-name`;
  const requestNameErrorID = `${widgetId}-request-name-error`;
  const requestTextID = `${widgetId}-request-text`;
  const requestTextHintID = `${widgetId}-request-text-hint`;
  const workTypeID = `${widgetId}-work-type`;
  const workTypeErrorID = `${widgetId}-work-type-error`;
  const statusID = `${widgetId}-status`;

  return (
    <DashboardWidgetFrame title={messages.cardTitle} widgetId={widgetId}>
      <form
        className={FORM_CLASS}
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={workTypeID}>
            {messages.workTypeLabel}
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
            <option value="">{messages.selectWorkTypePlaceholder}</option>
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
            {messages.requestNameLabel}
          </label>
          <Input
            aria-describedby={
              validationErrors?.requestName ? requestNameErrorID : undefined
            }
            aria-invalid={validationErrors?.requestName ? "true" : undefined}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            id={requestNameID}
            onChange={(event) => onRequestNameChange(event.target.value)}
            placeholder={messages.requestNamePlaceholder}
            type="text"
            value={draft.requestName}
          />
          {validationErrors?.requestName ? (
            <p className={VALIDATION_TEXT_CLASS} id={requestNameErrorID}>
              {validationErrors.requestName}
            </p>
          ) : null}
        </div>

        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={requestTextID}>
            {messages.requestLabel}
          </label>
          <Textarea
            aria-describedby={requestHint ? requestTextHintID : undefined}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            id={requestTextID}
            onChange={(event) => onRequestTextChange(event.target.value)}
            placeholder={messages.requestPlaceholder}
            value={draft.requestText}
          />
          {requestHint ? (
            <p className={HELP_TEXT_CLASS} id={requestTextHintID}>
              {requestHint}
            </p>
          ) : null}
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
            {isSubmitting ? messages.submittingAction : messages.submitAction}
          </Button>
        </div>
      </form>
    </DashboardWidgetFrame>
  );
}
