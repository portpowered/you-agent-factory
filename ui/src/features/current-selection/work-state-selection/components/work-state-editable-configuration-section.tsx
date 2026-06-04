import { type ReactNode, useId } from "react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import { CurrentSelectionExpandableSection } from "../../base/components/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/detail-card-factory-save-feedback";
import {
  CURRENT_SELECTION_FORM_FIELD_CLASS,
  CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS,
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
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.editableConfigurationHeading}
      toggleLabel={(expanded) =>
        expanded
          ? messages.editableConfigurationCollapseActionLabel
          : messages.editableConfigurationExpandActionLabel
      }
    >
      {state?.status === "loading" ? (
        <p
          className={cn(
            "m-0 text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
          {messages.editableConfigurationLoading}
        </p>
      ) : null}
      {state?.status === "error" ? (
        <p
          className={cn(
            "m-0 text-on-error-container",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          role="alert"
        >
          {messages.editableConfigurationErrorPrefix} {state.errorMessage}
        </p>
      ) : null}
      {state?.status === "empty" ? (
        <p
          className={cn(
            "m-0 text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
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
    </CurrentSelectionExpandableSection>
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
          className={cn(
            "m-0 text-on-error-container",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          role="alert"
        >
          {validationErrors.contract}
        </p>
      ) : null}
      <WorkStateEditableConfigurationDraftStatus
        messages={messages}
        state={state}
      />
      <div className={CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS}>
        <WorkStateEditableConfigurationField
          errorMessage={validationErrors.name}
          fieldId="editable-work-state-name"
          input={
            <input
              aria-describedby={
                validationErrors.name
                  ? "editable-work-state-name-error"
                  : undefined
              }
              aria-invalid={validationErrors.name ? "true" : undefined}
              className="w-full rounded-lg border border-outline bg-surface px-3 py-2 text-sm text-on-surface"
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
              className="block w-full rounded-lg border border-outline bg-surface-container-high px-3 py-2 text-sm text-on-surface-variant"
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
  if (!state.hasValidationErrors) {
    return null;
  }

  return (
    <div className={CURRENT_SELECTION_FORM_FIELD_CLASS}>
      <p
        className={cn("m-0 text-on-error-container", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationValidationStatus}
      </p>
      <p
        className={cn(
          "m-0 text-on-surface-subtle",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
      >
        {messages.editableConfigurationSaveDisabledValidationDetail}
      </p>
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
    <div className={CURRENT_SELECTION_FORM_FIELD_CLASS}>
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
      {errorMessage ? (
        <p
          className={cn(
            "m-0 text-on-error-container",
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
