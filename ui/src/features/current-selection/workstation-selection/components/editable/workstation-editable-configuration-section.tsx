// biome-ignore lint/style/noExcessiveLinesPerFile: current-selection editable workstation fields stay colocated so save feedback, overwrite hints, and responsive form structure evolve together.
import {
  EnumSelect,
  FormDescription,
  FormError,
} from "@you-agent-factory/components/forms";
import { surfacePanelVariants } from "@you-agent-factory/components/layout";
import { Label, Text } from "@you-agent-factory/components/primitives";
import { type ReactNode, useId } from "react";

import { AlertPanel, AlertPanelText } from "../../../../../components/ui/alert-panel";
import { Input } from "../../../../../components/ui/input";
import { formatList } from "../../../../../components/ui/formatters";
import { cn } from "../../../../../lib/cn";
import { isModelInvokeWorkstationType } from "../../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import type { EditableWorkstationType } from "../../../../current-factory-definition/lib/workstation/workstation-type";
import { supportsEditableWorkstationTypeConversion } from "../../../../current-factory-definition/lib/workstation/workstation-type";
import type { WorkstationLevelGuard } from "../../../../current-factory-definition/lib/workstation-guards";
import { workstationRequiresWorkerAssignment } from "../../../../current-factory-definition/lib/workstation-worker-assignment";
import { GraphSemanticIcon } from "../../../../flowchart/components/graph-semantic-icon";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../../base/components/save/detail-card-factory-save-feedback";
import {
  CurrentSelectionDetailFeedback,
} from "../../../base/components/detail/current-selection-detail-feedback";
import {
  CurrentSelectionFormField,
  CurrentSelectionFormFields,
} from "../../../base/components/layout/current-selection-form-layout";
import { formatEditableOverwriteFieldLabels } from "../../editing/editable-workstation-overwrite-fields";
import type {
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
  EditableWorkstationSaveValidationErrors,
  EditableWorkstationValidationErrors,
  WorkstationDetailCardProps,
  WorkstationSummaryItemProps,
  WorkstationSummaryProps,
} from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";
import { EditableConfigurationWorkstationGuardsField } from "../fields/workstation-guards-field";
import { EditableConfigurationWorkstationInputGuardsField } from "../fields/workstation-input-guards-field";
import { EditableConfigurationRunnerField } from "../fields/workstation-runner-field";
import {
  resolveWorkstationSummaryKindValue,
  resolveWorkstationSummaryPresentation,
  resolveWorkstationSummaryRequiresWorkerAssignment,
  resolveWorkstationSummaryRunnerValue,
  resolveWorkstationSummaryTypeValue,
} from "../fields/workstation-summary-field-values";
import { EditableConfigurationModelInvokeFields } from "../workstation-model-invoke-fields";
import { EditableConfigurationServerChangedHint } from "./editable-configuration-server-changed-hint";
import { EditableConfigurationCronFields } from "./workstation-cron-fields";
import { EditableConfigurationPromptInput } from "./workstation-prompt-field";

export function EditableConfigurationSection({
  messages,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  saveState?: EditableWorkstationSaveState;
  state?: WorkstationDetailCardProps["editableConfigurationState"];
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
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
        <EditableConfigurationReadyForm
          messages={messages}
          saveState={saveState}
          state={state}
        />
      ) : null}
    </CurrentSelectionExpandableSection>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: workstation ready form keeps save feedback, overwrite hints, and vertical field wiring colocated.
function EditableConfigurationReadyForm({
  messages,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  saveState?: EditableWorkstationSaveState;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const validationErrors = mergeDetailCardSaveFieldErrors<
    EditableWorkstationValidationErrors & Record<string, string | undefined>,
    EditableWorkstationSaveValidationErrors
  >(state.validationErrors, saveState);
  const renderState: typeof state = {
    ...state,
    validationErrors,
  };
  const requiresWorkerAssignment = workstationRequiresWorkerAssignment({
    type: state.draft.workstationType,
  });
  const isModelInvoke = isModelInvokeWorkstationType(
    state.draft.workstationType,
  );
  const showWorkstationTypeField =
    requiresWorkerAssignment &&
    supportsEditableWorkstationTypeConversion(
      state.initialValues.workstationType,
    );

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <EditableConfigurationOverwriteWarning
        messages={messages}
        overwriteFieldNames={state.overwriteFieldNames ?? []}
      />
      <EditableConfigurationDraftStatus messages={messages} state={state} />
      <EditableConfigurationField
        errorMessage={validationErrors.name}
        fieldId="editable-workstation-name"
        input={
          <Input
            aria-describedby={
              validationErrors.name
                ? "editable-workstation-name-error"
                : undefined
            }
            aria-invalid={validationErrors.name ? "true" : undefined}
            id="editable-workstation-name"
            onChange={(event) => state.onNameChange(event.target.value)}
            type="text"
            value={state.draft.name}
          />
        }
        label={messages.workstationNameFieldLabel}
        supportingContent={
          <EditableConfigurationServerChangedHint
            fieldName="name"
            messages={messages}
            state={state}
          />
        }
      />
      {showWorkstationTypeField ? (
        <EditableConfigurationField
          fieldId="editable-workstation-type"
          input={
            <EditableConfigurationWorkstationTypeInput
              messages={messages}
              state={renderState}
            />
          }
          label={messages.workstationTypeLabel}
          supportingContent={
            <EditableConfigurationServerChangedHint
              fieldName="workstationType"
              messages={messages}
              state={state}
            />
          }
        />
      ) : null}
      {isModelInvoke ? (
        <EditableConfigurationModelInvokeFields
          messages={messages}
          state={renderState}
          validationErrors={validationErrors}
        />
      ) : null}
      {requiresWorkerAssignment && !isModelInvoke ? (
        <CurrentSelectionFormFields>
          <EditableConfigurationField
            fieldId="editable-workstation-worker"
            errorMessage={validationErrors.workerName}
            input={
              <EditableConfigurationWorkerInput
                messages={messages}
                state={renderState}
              />
            }
            label={messages.workerFieldLabel}
            supportingContent={
              <>
                <EditableConfigurationSharedWorkerHint
                  messages={messages}
                  state={state}
                />
                <EditableConfigurationServerChangedHint
                  fieldName="worker"
                  messages={messages}
                  state={state}
                />
              </>
            }
          />
          <EditableConfigurationField
            fieldId="editable-workstation-kind"
            errorMessage={validationErrors.behavior}
            input={
              <EditableConfigurationBehaviorInput
                messages={messages}
                state={renderState}
              />
            }
            label={messages.kindLabel}
            supportingContent={
              <EditableConfigurationServerChangedHint
                fieldName="behavior"
                messages={messages}
                state={state}
              />
            }
          />
          <EditableConfigurationCronFields
            messages={messages}
            state={renderState}
          />
          <EditableConfigurationField
            fieldId="editable-workstation-runner"
            errorMessage={validationErrors.runnerName}
            input={
              <EditableConfigurationRunnerField
                messages={messages}
                state={renderState}
              />
            }
            label={messages.runnerFieldLabel}
            supportingContent={
              <EditableConfigurationServerChangedHint
                fieldName="runner"
                messages={messages}
                state={state}
              />
            }
          />
          <EditableConfigurationField
            errorMessage={validationErrors.prompt}
            fieldId="editable-workstation-prompt"
            input={
              <EditableConfigurationPromptInput
                messages={messages}
                state={renderState}
              />
            }
            label={messages.promptFieldLabel}
            supportingContent={
              <EditableConfigurationServerChangedHint
                fieldName="prompt"
                messages={messages}
                state={state}
              />
            }
          />
        </CurrentSelectionFormFields>
      ) : null}
      <EditableConfigurationWorkstationGuardsField
        fieldErrors={validationErrors}
        guards={state.draft.guards as WorkstationLevelGuard[]}
        messages={messages}
        onGuardsChange={state.onGuardsChange}
        workstationOptionsState={state.workstationOptionsState}
      />
      <EditableConfigurationWorkstationInputGuardsField
        fieldErrors={validationErrors}
        inputs={state.draft.inputs}
        messages={messages}
        onInputsChange={state.onInputsChange}
      />
    </form>
  );
}

function hasOnlyPromptBlockingValidationErrors(
  validationErrors: EditableWorkstationValidationErrors,
  promptDiagnostics: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptDiagnostics"],
): boolean {
  return (
    promptDiagnostics.length > 0 &&
    Boolean(validationErrors.prompt) &&
    !validationErrors.behavior &&
    !validationErrors.cronExpiryWindow &&
    !validationErrors.cronJitter &&
    !validationErrors.cronSchedule &&
    !validationErrors.cronTriggerAtStart &&
    !validationErrors.name &&
    !validationErrors.runnerName &&
    !validationErrors.workerName
  );
}

function EditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const promptOnlyValidationErrors = hasOnlyPromptBlockingValidationErrors(
    state.validationErrors,
    state.promptDiagnostics,
  );

  if (!state.hasValidationErrors || promptOnlyValidationErrors) {
    return null;
  }

  return (
    <CurrentSelectionDetailFeedback role="alert" tone="danger">
      {messages.editableConfigurationValidationStatus}
    </CurrentSelectionDetailFeedback>
  );
}

function EditableConfigurationOverwriteWarning({
  messages,
  overwriteFieldNames,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  overwriteFieldNames?: EditableWorkstationOverwriteField[];
}) {
  if (!overwriteFieldNames || overwriteFieldNames.length === 0) {
    return null;
  }

  const formattedFields = formatEditableOverwriteFieldLabels(
    overwriteFieldNames,
    messages,
  );

  return (
    <AlertPanel tone="warning">
      <AlertPanelText role="alert">
        {messages.editableConfigurationOverwriteWarning(formattedFields)}
      </AlertPanelText>
    </AlertPanel>
  );
}

function EditableConfigurationWorkerInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  if (state.workerOptionsState.status === "empty") {
    return (
      <FormDescription variant="body">
        {state.workerOptionsState.message}
      </FormDescription>
    );
  }

  if (state.workerOptionsState.status === "error") {
    return (
      <FormError>
        {messages.editableConfigurationWorkerUnavailablePrefix}{" "}
        {state.workerOptionsState.message}
      </FormError>
    );
  }

  return (
    <EnumSelect
      aria-describedby={
        state.validationErrors.workerName
          ? "editable-workstation-worker-error"
          : undefined
      }
      aria-invalid={state.validationErrors.workerName ? "true" : undefined}
      aria-label={messages.workerFieldLabel}
      id="editable-workstation-worker"
      onValueChange={state.onWorkerChange}
      options={state.workerOptionsState.options.map((workerName) => ({
        label: valueOrFallback(workerName, messages.notConfiguredValue),
        value: workerName,
      }))}
      value={state.draft.workerName}
    />
  );
}

function EditableConfigurationWorkstationTypeInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  return (
    <EnumSelect
      aria-label={messages.workstationTypeLabel}
      id="editable-workstation-type"
      onValueChange={(nextValue) =>
        state.onWorkstationTypeChange(nextValue as EditableWorkstationType)
      }
      options={state.initialValues.workstationTypeOptions.map(
        (workstationType) => ({
          label: messages.localizeWorkstationType(workstationType),
          value: workstationType,
        }),
      )}
      value={state.draft.workstationType}
    />
  );
}

function EditableConfigurationBehaviorInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const behaviorHintId =
    state.draft.behavior === "POLLER"
      ? "editable-workstation-kind-hint"
      : undefined;

  return (
    <>
      <EnumSelect
        aria-describedby={
          [
            behaviorHintId,
            state.validationErrors.behavior
              ? "editable-workstation-kind-error"
              : undefined,
          ]
            .filter(Boolean)
            .join(" ") || undefined
        }
        aria-invalid={state.validationErrors.behavior ? "true" : undefined}
        aria-label={messages.kindLabel}
        id="editable-workstation-kind"
        onValueChange={(nextValue) =>
          state.onBehaviorChange(nextValue as typeof state.draft.behavior)
        }
        options={state.initialValues.behaviorOptions.map((behavior) => ({
          label: messages.localizeWorkstationBehavior(behavior),
          value: behavior,
        }))}
        value={state.draft.behavior}
      />
      {state.draft.behavior === "POLLER" ? (
        <FormDescription id={behaviorHintId}>
          {messages.editableConfigurationBehaviorPollerHint}
        </FormDescription>
      ) : null}
    </>
  );
}

function EditableConfigurationSharedWorkerHint({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const sharedWorkstationNames = resolveSharedWorkerWorkstationNames(state);
  if (sharedWorkstationNames.length === 0) {
    return null;
  }

  return (
    <Text className="m-0 text-on-surface-subtle" variant="supporting">
      {messages.editableConfigurationSharedWorkerScopeHint(
        valueOrFallback(state.draft.workerName, messages.notConfiguredValue),
        formatList(sharedWorkstationNames),
      )}
    </Text>
  );
}

function resolveSharedWorkerWorkstationNames(
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >,
): string[] {
  return (
    state.initialValues.sharedWorkerWorkstationNamesByWorkerName[
      state.draft.workerName
    ] ?? []
  );
}

export function WorkstationSummary({
  activeRunCount,
  editableConfigurationState,
  historyCount,
  historyLabel,
  locale,
  messages,
  selectedNode,
}: WorkstationSummaryProps) {
  const sectionId = `workstation-summary-${selectedNode.node_id}`;
  const requiresWorkerAssignment =
    resolveWorkstationSummaryRequiresWorkerAssignment(
      editableConfigurationState,
      selectedNode,
    );
  const summaryRunnerValue = resolveWorkstationSummaryRunnerValue(
    editableConfigurationState,
    messages,
    selectedNode,
  );
  const summaryKindValue = resolveWorkstationSummaryKindValue(
    editableConfigurationState,
    selectedNode,
    messages,
  );
  const summaryKindPresentation = resolveWorkstationSummaryPresentation(
    editableConfigurationState,
    selectedNode,
    locale,
  );

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      headingId={sectionId}
      title={messages.summaryHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <ul className="m-0 grid list-none gap-2 p-0 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
        {requiresWorkerAssignment ? (
          <WorkstationSummaryItem
            label={messages.workerTypeLabel}
            value={selectedNode.worker_type || messages.unknownWorkerTypeValue}
          />
        ) : null}
        <WorkstationSummaryItem
          label={messages.workstationTypeLabel}
          value={resolveWorkstationSummaryTypeValue(
            editableConfigurationState,
            messages,
          )}
        />
        {summaryRunnerValue != null ? (
          <WorkstationSummaryItem
            label={messages.selectedRunnerLabel}
            value={summaryRunnerValue}
          />
        ) : null}
        {summaryKindValue != null ? (
          <WorkstationSummaryItem
            iconClassName={summaryKindPresentation?.className}
            iconKind={summaryKindPresentation?.iconKind}
            iconLabel={summaryKindPresentation?.label}
            label={messages.kindLabel}
            value={summaryKindValue}
          />
        ) : null}
        <WorkstationSummaryItem
          label={messages.inputWorkTypesLabel}
          value={formatList(selectedNode.input_work_type_ids)}
        />
        <WorkstationSummaryItem
          label={messages.outputWorkTypesLabel}
          value={formatList(selectedNode.output_work_type_ids)}
        />
        <WorkstationSummaryItem
          label={messages.activeRunsLabel}
          value={activeRunCount}
        />
        <WorkstationSummaryItem label={historyLabel} value={historyCount} />
      </ul>
    </CurrentSelectionExpandableSection>
  );
}

function EditableConfigurationField({
  errorMessage,
  fieldId,
  input,
  label,
  supportingContent,
}: {
  errorMessage?: string;
  fieldId: string;
  input: ReactNode;
  label: string;
  supportingContent?: ReactNode;
}) {
  return (
    <CurrentSelectionFormField>
      <Label as="label" htmlFor={fieldId}>
        {label}
      </Label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <CurrentSelectionDetailFeedback id={`${fieldId}-error`} tone="danger">
          {errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
    </CurrentSelectionFormField>
  );
}

function WorkstationSummaryItem({
  iconClassName,
  iconKind,
  iconLabel,
  label,
  value,
}: WorkstationSummaryItemProps) {
  return (
    <li
      className={surfacePanelVariants({
        className: "grid min-w-0 gap-1 px-3 py-2",
        radius: "lg",
      })}
      data-workstation-summary-item={label}
    >
      <Label>{label}</Label>
      <strong className="flex min-w-0 items-center gap-2 text-sm text-on-surface [overflow-wrap:anywhere]">
        {iconKind && iconLabel ? (
          <GraphSemanticIcon
            className={cn("h-4 w-4 shrink-0", iconClassName)}
            kind={iconKind}
            label={iconLabel}
          />
        ) : null}
        <span className="min-w-0">{value}</span>
      </strong>
    </li>
  );
}

function valueOrFallback(value: string | null, fallback: string) {
  return value && value.trim().length > 0 ? value : fallback;
}
