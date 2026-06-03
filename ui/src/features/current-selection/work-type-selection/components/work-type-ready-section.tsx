import { type ReactNode, useId, useState } from "react";

import {
  DashboardStatusPill,
  ExpandablePanelTrigger,
} from "../../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import type { EditableWorkTypeValidationErrors } from "../../../current-factory-definition/lib/work-type-editable-validation";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/detail-card-factory-save-feedback";
import {
  CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS,
  CURRENT_SELECTION_FORM_FIELD_CLASS,
  CURRENT_SELECTION_NOTICE_SUBTLE_CLASS,
  CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS,
  CurrentSelectionSectionHeader,
} from "../../base/components/detail-card-shared";
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
  const [expanded, setExpanded] = useState(true);
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <section
      aria-labelledby={headingId}
      className="mt-0 grid gap-2.5 [&_h4]:m-0"
    >
      <CurrentSelectionSectionHeader
        action={
          <ExpandablePanelTrigger
            aria-label={
              expanded
                ? messages.editableConfigurationCollapseActionLabel
                : messages.editableConfigurationExpandActionLabel
            }
            controlsID={contentId}
            expanded={expanded}
            onClick={() => setExpanded((current) => !current)}
            type="button"
            variant="section"
          >
            {expanded ? messages.collapseAction : messages.expandAction}
          </ExpandablePanelTrigger>
        }
        headingId={headingId}
        title={messages.editableConfigurationHeading}
      />
      {expanded ? (
        <div
          className={CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS}
          id={contentId}
        >
          <WorkTypeReadySection
            messages={messages}
            onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
            saveState={saveState}
            state={state}
            workTypeName={workTypeName}
          />
        </div>
      ) : null}
    </section>
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
                validationErrors.name
                  ? "editable-work-type-name-error"
                  : undefined
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
        <div className={CURRENT_SELECTION_FORM_FIELD_CLASS}>
          {hasDefaultHandlingBehavior ? (
            <DashboardStatusPill
              id={handlingBehaviorStatusId}
              role="status"
              tone="active"
            >
              {messages.handlingBehaviorDefaultStatusLabel}
            </DashboardStatusPill>
          ) : null}
          <label
            className={cn(
              "flex items-center gap-2",
              DASHBOARD_SUPPORTING_LABEL_CLASS,
            )}
            htmlFor="editable-work-type-handling-behavior-default"
          >
            <input
              aria-describedby={handlingBehaviorDescribedBy || undefined}
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
          {!hasDefaultHandlingBehavior ? (
            <p
              className={CURRENT_SELECTION_NOTICE_SUBTLE_CLASS}
              id={handlingBehaviorHelperId}
            >
              {messages.handlingBehaviorDefaultHelper}
            </p>
          ) : null}
          {validationErrors.handlingBehavior ? (
            <p
              className={cn(
                "m-0 text-af-danger-text",
                DASHBOARD_BODY_TEXT_CLASS,
              )}
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
    <div className={CURRENT_SELECTION_FORM_FIELD_CLASS}>
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
