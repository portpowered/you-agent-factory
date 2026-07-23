import { type ReactNode, useId } from "react";

import {
  Checkbox,
  DashboardStatusPill,
  Input,
  Label,
} from "../../../../components/ui";
import type { EditableWorkTypeValidationErrors } from "../../../current-factory-definition/lib/work-type-editable-validation";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/save/detail-card-factory-save-feedback";
import {
  CurrentSelectionDetailFeedback,
  CurrentSelectionFormField,
  CurrentSelectionFormFields,
  CurrentSelectionSupportingText,
} from "../../base/public";
import type {
  EditableWorkTypeConfigurationState,
  EditableWorkTypeSaveState,
  EditableWorkTypeSaveValidationErrors,
} from "../lib/detail-card-types";
import type { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeStatesList } from "./work-type-states-list";

export function WorkTypeEditableConfigurationSection({
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
      <WorkTypeReadySection
        messages={messages}
        onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
        saveState={saveState}
        state={state}
        workTypeName={workTypeName}
      />
    </CurrentSelectionExpandableSection>
  );
}

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
  const handlingBehaviorHelperId =
    "editable-work-type-handling-behavior-helper";
  const handlingBehaviorStatusId =
    "editable-work-type-handling-behavior-status";
  const handlingBehaviorDescribedBy = [
    hasDefaultHandlingBehavior
      ? handlingBehaviorStatusId
      : handlingBehaviorHelperId,
    validationErrors.handlingBehavior
      ? "editable-work-type-handling-behavior-error"
      : undefined,
  ]
    .filter((id): id is string => id != null)
    .join(" ");

  return (
    <form className="grid gap-2.5" onSubmit={(event) => event.preventDefault()}>
      {validationErrors.contract ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {validationErrors.contract}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state.hasValidationErrors ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {messages.editableConfigurationValidationStatus}
        </CurrentSelectionDetailFeedback>
      ) : null}
      <CurrentSelectionFormFields>
        <WorkTypeEditableField
          errorMessage={validationErrors.name}
          fieldId="editable-work-type-name"
          input={
            <Input
              aria-describedby={
                validationErrors.name
                  ? "editable-work-type-name-error"
                  : undefined
              }
              aria-invalid={validationErrors.name ? "true" : undefined}
              id="editable-work-type-name"
              onChange={(event) => state.onNameChange(event.target.value)}
              type="text"
              value={state.draft.name}
            />
          }
          label={messages.workTypeNameLabel}
        />
        <CurrentSelectionFormField>
          {hasDefaultHandlingBehavior ? (
            <DashboardStatusPill
              id={handlingBehaviorStatusId}
              role="status"
              tone="active"
            >
              {messages.handlingBehaviorDefaultStatusLabel}
            </DashboardStatusPill>
          ) : null}
          <Label
            as="label"
            className="flex items-center gap-2"
            htmlFor="editable-work-type-handling-behavior-default"
          >
            <Checkbox
              aria-describedby={handlingBehaviorDescribedBy || undefined}
              aria-invalid={
                validationErrors.handlingBehavior ? "true" : undefined
              }
              checked={hasDefaultHandlingBehavior}
              id="editable-work-type-handling-behavior-default"
              onChange={(event) =>
                state.onHandlingBehaviorChange(
                  event.target.checked ? ["DEFAULT"] : null,
                )
              }
            />
            {messages.handlingBehaviorDefaultLabel}
          </Label>
          {!hasDefaultHandlingBehavior ? (
            <CurrentSelectionSupportingText id={handlingBehaviorHelperId}>
              {messages.handlingBehaviorDefaultHelper}
            </CurrentSelectionSupportingText>
          ) : null}
          {validationErrors.handlingBehavior ? (
            <CurrentSelectionDetailFeedback
              id="editable-work-type-handling-behavior-error"
              role="alert"
              tone="danger"
            >
              {validationErrors.handlingBehavior}
            </CurrentSelectionDetailFeedback>
          ) : null}
        </CurrentSelectionFormField>
        <WorkTypeStatesList
          messages={messages}
          onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
          states={state.initialValues.states}
          workTypeName={workTypeName}
        />
      </CurrentSelectionFormFields>
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
