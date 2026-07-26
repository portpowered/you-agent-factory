import { type ReactNode, useId } from "react";

import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Label, Text } from "@you-agent-factory/components/primitives";
import { Input } from "../../../../components/ui/input";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/save/detail-card-factory-save-feedback";
import { CurrentSelectionDetailFeedback } from "../../base/components/detail/current-selection-detail-feedback";
import {
  CurrentSelectionFormField,
  CurrentSelectionFormFields,
} from "../../base/components/layout/current-selection-form-layout";
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
        <CurrentSelectionDetailFeedback>
          {messages.editableConfigurationLoading}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "error" ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {messages.editableConfigurationErrorPrefix} {state.errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "empty" ? (
        <CurrentSelectionDetailFeedback>
          {state.message || messages.editableConfigurationEmpty}
        </CurrentSelectionDetailFeedback>
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
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {validationErrors.contract}
        </CurrentSelectionDetailFeedback>
      ) : null}
      <WorkStateEditableConfigurationDraftStatus
        messages={messages}
        state={state}
      />
      <CurrentSelectionFormFields>
        <WorkStateEditableConfigurationField
          errorMessage={validationErrors.name}
          fieldId="editable-work-state-name"
          input={
            <Input
              aria-describedby={
                validationErrors.name
                  ? "editable-work-state-name-error"
                  : undefined
              }
              aria-invalid={validationErrors.name ? "true" : undefined}
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
            <SurfacePanel
              asChild
              className="text-sm text-on-surface-variant"
              padding="compact"
              radius="lg"
            >
              <output id="editable-work-state-type">
                {messages.localizeWorkStateType(state.draft.type)}
              </output>
            </SurfacePanel>
          }
          label={messages.typeFieldLabel}
        />
      </CurrentSelectionFormFields>
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
    <CurrentSelectionFormField>
      <CurrentSelectionDetailFeedback role="alert" tone="danger">
        {messages.editableConfigurationValidationStatus}
      </CurrentSelectionDetailFeedback>
      <Text className="m-0 text-on-surface-subtle" variant="supporting">
        {messages.editableConfigurationSaveDisabledValidationDetail}
      </Text>
    </CurrentSelectionFormField>
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
    <CurrentSelectionFormField>
      <Label as="label" htmlFor={fieldId}>
        {label}
      </Label>
      {input}
      {errorMessage ? (
        <CurrentSelectionDetailFeedback
          id={`${fieldId}-error`}
          role="alert"
          tone="danger"
        >
          {errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
    </CurrentSelectionFormField>
  );
}
