import type { ReactNode } from "react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import {
  mergeDetailCardSaveFieldErrors,
} from "../../base/components/detail-card-factory-save-feedback";
import {
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS,
} from "../../base/components/detail-card-shared";
import type {
  EditableWorkTypeConfigurationState,
  EditableWorkTypeSaveState,
  EditableWorkTypeSaveValidationErrors,
} from "../lib/detail-card-types";
import type { EditableWorkTypeValidationErrors } from "../../../current-factory-definition/lib/work-type-editable-validation";
import type { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeStatesList } from "./work-type-states-list";

export function WorkTypeReadySection({
  messages,
  onSelectWorkStateGraphNode,
  saveState,
  state,
  workTypeName,
}: {
  messages: ReturnType<typeof getWorkTypeDetailMessages>;
  onSelectWorkStateGraphNode?: (graphNodeId: string) => void;
  saveState?: EditableWorkTypeSaveState;
  state: Extract<EditableWorkTypeConfigurationState, { status: "ready" }>;
  workTypeName: string;
}) {
  const validationErrors = mergeDetailCardSaveFieldErrors<
    EditableWorkTypeValidationErrors & Record<string, string | undefined>,
    EditableWorkTypeSaveValidationErrors
  >(state.validationErrors, saveState);
  const hasDefaultHandlingBehavior =
    state.draft.handlingBehavior?.includes("DEFAULT") ?? false;

  return (
    <form className="grid gap-2.5" onSubmit={(event) => event.preventDefault()}>
      {validationErrors.contract ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {validationErrors.contract}
        </p>
      ) : null}
      {state.hasValidationErrors ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {messages.editableConfigurationValidationStatus}
        </p>
      ) : null}
      <div className={CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS}>
        <WorkTypeEditableField
          errorMessage={validationErrors.name}
          fieldId="editable-work-type-name"
          input={
            <input
              aria-describedby={
                validationErrors.name ? "editable-work-type-name-error" : undefined
              }
              aria-invalid={validationErrors.name ? "true" : undefined}
              className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
              id="editable-work-type-name"
              onChange={(event) => state.onNameChange(event.target.value)}
              type="text"
              value={state.draft.name}
            />
          }
          label={messages.workTypeNameLabel}
        />
        <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
          <label
            className={cn(
              "flex items-center gap-2",
              DASHBOARD_SUPPORTING_LABEL_CLASS,
            )}
            htmlFor="editable-work-type-handling-behavior-default"
          >
            <input
              aria-describedby={
                validationErrors.handlingBehavior
                  ? "editable-work-type-handling-behavior-error"
                  : undefined
              }
              aria-invalid={
                validationErrors.handlingBehavior ? "true" : undefined
              }
              checked={hasDefaultHandlingBehavior}
              className="size-4 rounded border border-af-border"
              id="editable-work-type-handling-behavior-default"
              onChange={(event) =>
                state.onHandlingBehaviorChange(
                  event.target.checked ? ["DEFAULT"] : null,
                )
              }
              type="checkbox"
            />
            {messages.handlingBehaviorDefaultLabel}
          </label>
          {validationErrors.handlingBehavior ? (
            <p
              className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
              id="editable-work-type-handling-behavior-error"
              role="alert"
            >
              {validationErrors.handlingBehavior}
            </p>
          ) : null}
        </div>
        <WorkTypeStatesList
          messages={messages}
          onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
          states={state.initialValues.states}
          workTypeName={workTypeName}
        />
      </div>
    </form>
  );
}

function WorkTypeEditableField({
  errorMessage,
  fieldId,
  input,
  label,
}: {
  errorMessage?: string;
  fieldId: string;
  input: ReactNode;
  label: string;
}) {
  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
      {errorMessage ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          id={`${fieldId}-error`}
          role="alert"
        >
          {errorMessage}
        </p>
      ) : null}
    </div>
  );
}
