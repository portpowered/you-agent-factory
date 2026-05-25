import type { ReactNode } from "react";

import {
  Button,
  DashboardWidgetFrame,
  Input,
  Select,
  Textarea,
} from "../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { getSubmitWorkMessages } from "../messages/submit-work";

export interface SubmitWorkDraft {
  items: SubmitWorkDraftItem[];
  requestName: string;
  workTypeName: string;
}

export interface SubmitWorkDraftTextItem {
  id: string;
  text: string;
  type: "text";
}

export type SubmitWorkDraftItem = SubmitWorkDraftTextItem;

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
  headerAction?: ReactNode;
  isSubmitting?: boolean;
  locale?: string;
  onItemTextChange: (itemId: string, value: string) => void;
  onRequestNameChange: (value: string) => void;
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
const ACTION_ROW_CLASS = "mt-auto flex items-start gap-3";
const HELP_TEXT_CLASS = cn(
  "max-w-xl leading-relaxed text-af-text-muted",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const VALIDATION_TEXT_CLASS = cn(
  "text-af-danger-text",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const STATUS_TONE_CLASS_BY_KIND: Record<SubmitWorkStatus["kind"], string> = {
  error: "text-af-danger-text",
  guidance: "text-af-text-subtle",
  submitting: "text-af-text",
  success: "text-af-success-text",
  "validation-error": "text-af-danger-text",
};

export function SubmitWorkCard({
  draft,
  headerAction,
  isSubmitting = false,
  locale,
  onItemTextChange,
  onRequestNameChange,
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
  const requestNameID = `${widgetId}-request-name`;
  const requestNameErrorID = `${widgetId}-request-name-error`;
  const submissionItemsID = `${widgetId}-submission-items`;
  const workTypeID = `${widgetId}-work-type`;
  const workTypeErrorID = `${widgetId}-work-type-error`;
  const statusID = `${widgetId}-status`;

  return (
    <DashboardWidgetFrame
      headerAction={headerAction}
      title={messages.cardTitle}
      widgetId={widgetId}
    >
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
          <div className={FIELD_LABEL_CLASS} id={submissionItemsID}>
            {messages.submissionItemsLabel}
          </div>
          <ol
            aria-labelledby={submissionItemsID}
            className="grid gap-3"
          >
            {draft.items.map((item, index) => {
              const requestTextID = `${widgetId}-${item.id}-text`;
              const requestTextHintID = `${widgetId}-${item.id}-text-hint`;
              const requestItemLabel = messages.requestItemLabel(index + 1);
              const requestHint = messages.requestHint?.trim();

              return (
                <li
                  className="grid gap-2 rounded-lg border border-af-border-subtle bg-af-panel p-3"
                  key={item.id}
                >
                  <div className="flex items-center justify-between gap-3">
                    <span className={FIELD_LABEL_CLASS}>
                      {messages.textItemTypeLabel}
                    </span>
                    <span className={HELP_TEXT_CLASS}>{requestItemLabel}</span>
                  </div>
                  <label className={FIELD_LABEL_CLASS} htmlFor={requestTextID}>
                    {requestItemLabel}
                  </label>
                  <Textarea
                    aria-describedby={requestHint ? requestTextHintID : undefined}
                    className={DASHBOARD_BODY_TEXT_CLASS}
                    disabled={controlsDisabled}
                    id={requestTextID}
                    onChange={(event) =>
                      onItemTextChange(item.id, event.target.value)
                    }
                    placeholder={messages.requestPlaceholder}
                    value={item.text}
                  />
                  {requestHint ? (
                    <p className={HELP_TEXT_CLASS} id={requestTextHintID}>
                      {requestHint}
                    </p>
                  ) : null}
                </li>
              );
            })}
          </ol>
        </div>

        <div className={ACTION_ROW_CLASS}>
          <p
            className={cn(
              "min-w-0 flex-1",
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
            className="ml-auto shrink-0 self-start"
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
