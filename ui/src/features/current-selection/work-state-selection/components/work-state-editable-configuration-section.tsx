import type { ReactNode } from "react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import {
  mergeDetailCardSaveFieldErrors,
} from "../../base/components/detail-card-factory-save-feedback";
import {
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS,
  CurrentSelectionSectionHeader,
} from "../../base/components/detail-card-shared";
import type {
  EditableWorkStateConfigurationState,
  EditableWorkStateSaveState,
  EditableWorkStateSaveValidationErrors,
} from "../lib/detail-card-types";
import type { EditableWorkStateValidationErrors } from "../lib/work-state-editable-validation";
import type { getWorkStateDetailMessages } from "../messages/work-state-detail";

export function WorkStateEditableConfigurationSection({
  messages,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getWorkStateDetailMessages>;
  saveState?: EditableWorkStateSaveState;
  state?: EditableWorkStateConfigurationState;
}) {
  const headingId = "editable-work-state-configuration-heading";

  return (
    <section aria-labelledby={headingId} className="mt-0 grid gap-2.5 [&_h4]:m-0">
      <CurrentSelectionSectionHeader
        headingId={headingId}
        title={messages.editableConfigurationHeading}
      />
      <div className="grid gap-2.5">
        {state?.status === "loading" ? (
          <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
            {messages.editableConfigurationLoading}
          </p>
        ) : null}
        {state?.status === "error" ? (
          <p
            className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
            role="alert"
          >
            {messages.editableConfigurationErrorPrefix} {state.errorMessage}
          </p>
        ) : null}
        {state?.status === "empty" ? (
          <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
            {state.message || messages.editableConfigurationEmpty}
          </p>
        ) : null}
        {state?.status === "ready" ? (
          <WorkStateEditableConfigurationReadyForm
            messages={messages}
            saveState={saveState}
            state={state}
          />
        ) : null}
      </div>
    </section>
  );
}

function WorkStateEditableConfigurationReadyForm({
  messages,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getWorkStateDetailMessages>;
  saveState?: EditableWorkStateSaveState;
  state: Extract<EditableWorkStateConfigurationState, { status: "ready" }>;
}) {
  const validationErrors = mergeDetailCardSaveFieldErrors<
    EditableWorkStateValidationErrors & Record<string, string | undefined>,
    EditableWorkStateSaveValidationErrors
  >(state.validationErrors, saveState);

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      {validationErrors.contract ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {validationErrors.contract}
        </p>
      ) : null}
      <WorkStateEditableConfigurationDraftStatus messages={messages} state={state} />
      <div className={CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS}>
        <WorkStateEditableConfigurationField
          errorMessage={validationErrors.name}
          fieldId="editable-work-state-name"
          input={
            <input
              aria-describedby={
                validationErrors.name ? "editable-work-state-name-error" : undefined
              }
              aria-invalid={validationErrors.name ? "true" : undefined}
              className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
              id="editable-work-state-name"
              onChange={(event) => state.onNameChange(event.target.value)}
              type="text"
              value={state.draft.name}
            />
          }
          label={messages.nameFieldLabel}
        />
        <WorkStateEditableConfigurationField
          fieldId="editable-work-state-type"
          input={
            <output
              className="block w-full rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2 text-sm text-af-text-muted"
              id="editable-work-state-type"
            >
              {messages.localizeWorkStateType(state.draft.type)}
            </output>
          }
          label={messages.typeFieldLabel}
        />
      </div>
    </form>
  );
}

function WorkStateEditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkStateDetailMessages>;
  state: Extract<EditableWorkStateConfigurationState, { status: "ready" }>;
}) {
  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <p
        className={cn(
          "m-0",
          state.hasValidationErrors
            ? "text-af-danger-text"
            : "text-af-text-muted",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
        role={state.hasValidationErrors ? "alert" : "status"}
      >
        {state.hasValidationErrors
          ? messages.editableConfigurationValidationStatus
          : state.isDirty
            ? messages.editableConfigurationDirtyStatus
            : messages.editableConfigurationDraftNote}
      </p>
      {state.hasValidationErrors ? (
        <p
          className={cn(
            "m-0 text-af-text-subtle",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.editableConfigurationSaveDisabledValidationDetail}
        </p>
      ) : (
        <p
          className={cn(
            "m-0 text-af-text-subtle",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.editableConfigurationDraftNote}
        </p>
      )}
    </div>
  );
}

function WorkStateEditableConfigurationField({
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
          className={cn(
            "m-0 text-af-danger-text",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
          id={`${fieldId}-error`}
          role="alert"
        >
          {errorMessage}
        </p>
      ) : null}
    </div>
  );
}
