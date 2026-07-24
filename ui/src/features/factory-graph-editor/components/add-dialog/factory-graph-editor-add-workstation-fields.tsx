import { WorkstationKind } from "../../../../api/generated/openapi";
import { Checkbox } from "../../../../components/ui";
import type { EditableWorkstationType } from "../../../current-factory-definition/lib/workstation/workstation-type";
import {
  type EditableWorkstationBehavior,
  workstationBehaviorRequiresPrompt,
} from "../../../current-factory-definition/lib/workstation-behavior";
import type { EditableWorkstationCronDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";
import { FACTORY_GRAPH_ADD_WORKSTATION_TYPES } from "../../../current-factory-definition/public";
import { getWorkstationDetailMessages } from "../../../current-selection/workstation-selection/messages/workstation-detail";
import type { CanonicalFactoryDefinition } from "../../lib/draft/factory-graph-draft-types";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
} from "../../lib/editor/factory-graph-editor-additions";
import {
  editableWorkstationBehaviorOptions,
  resolveFactoryGraphAddWorkstationDraftForBehaviorChange,
  resolveFactoryGraphAddWorkstationDraftForTypeChange,
} from "../../lib/editor/factory-graph-editor-additions";
import type { getFactoryGraphEditorMessages } from "../../messages/editor";
import {
  FactoryGraphEditorAddField,
  FactoryGraphEditorPromptBodyField,
  FactoryGraphEditorSelectField,
  FactoryGraphEditorTextField,
} from "./factory-graph-editor-add-dialog-fields";

const FACTORY_GRAPH_ADD_WORKSTATION_TYPE_OPTIONS =
  FACTORY_GRAPH_ADD_WORKSTATION_TYPES;

export function FactoryGraphEditorAddWorkstationFields({
  currentFactoryDefinition,
  draft,
  errors,
  locale,
  messages,
  onChange,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: Extract<FactoryGraphAddEntityDraft, { kind: "workstation" }>;
  errors: FactoryGraphAddEntityFieldErrors;
  locale?: string;
  messages: ReturnType<typeof getFactoryGraphEditorMessages>;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
}) {
  const workstationMessages = getWorkstationDetailMessages(locale);
  const requiresWorkerAssignment = workstationRequiresWorkerAssignment({
    type: draft.workstationType,
  });
  const showPrompt =
    requiresWorkerAssignment &&
    workstationBehaviorRequiresPrompt(draft.behavior);

  return (
    <>
      <FactoryGraphEditorSelectField
        inputId="factory-graph-add-workstation-type"
        label={workstationMessages.workstationTypeLabel}
        onChange={(value) => {
          const workstationType = value as EditableWorkstationType;
          onChange(
            resolveFactoryGraphAddWorkstationDraftForTypeChange(
              draft,
              workstationType,
              {
                defaultWorkerName: currentFactoryDefinition?.workers?.[0]?.name,
              },
            ),
          );
        }}
        options={FACTORY_GRAPH_ADD_WORKSTATION_TYPE_OPTIONS.map(
          (workstationType) => ({
            label: workstationMessages.localizeWorkstationType(workstationType),
            value: workstationType,
          }),
        )}
        value={draft.workstationType}
      />
      <FactoryGraphEditorSelectField
        error={errors.behavior}
        inputId="factory-graph-add-workstation-kind"
        label={messages.addDialogKindLabel}
        onChange={(value) => {
          onChange(
            resolveFactoryGraphAddWorkstationDraftForBehaviorChange(
              draft,
              value as EditableWorkstationBehavior,
            ),
          );
        }}
        options={editableWorkstationBehaviorOptions().map((behavior) => ({
          label: workstationMessages.localizeWorkstationBehavior(behavior),
          value: behavior,
        }))}
        value={draft.behavior}
      />
      {draft.behavior === WorkstationKind.CRON && draft.cron ? (
        <FactoryGraphEditorAddWorkstationCronFields
          cron={draft.cron}
          errors={errors}
          messages={workstationMessages}
          onChange={(cron) => {
            onChange({ ...draft, cron });
          }}
        />
      ) : null}
      {requiresWorkerAssignment ? (
        <>
          <FactoryGraphEditorSelectField
            error={errors.workerName}
            inputId="factory-graph-add-worker-name"
            label={messages.addDialogAssignedWorkerLabel}
            onChange={(value) => {
              onChange({ ...draft, workerName: value });
            }}
            options={[
              {
                label: messages.addDialogAssignedWorkerPlaceholder,
                value: "",
              },
              ...(currentFactoryDefinition?.workers ?? []).map((worker) => ({
                label: worker.name,
                value: worker.name,
              })),
            ]}
            value={draft.workerName}
          />
          {showPrompt ? (
            <FactoryGraphEditorPromptBodyField
              helpText={messages.addDialogPromptBodyHelp}
              label={messages.addDialogPromptBodyLabel}
              loadingMessage={messages.addDialogPromptBodyEditorLoading}
              onChange={(value) => {
                onChange({ ...draft, body: value });
              }}
              startupErrorMessage={messages.addDialogPromptBodyEditorError}
              value={draft.body}
            />
          ) : null}
        </>
      ) : null}
    </>
  );
}

function FactoryGraphEditorAddWorkstationCronFields({
  cron,
  errors,
  messages,
  onChange,
}: {
  cron: EditableWorkstationCronDraft;
  errors: FactoryGraphAddEntityFieldErrors;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (cron: EditableWorkstationCronDraft) => void;
}) {
  return (
    <>
      <FactoryGraphEditorTextField
        error={errors.cronSchedule}
        helpText={messages.cronScheduleFieldHint}
        inputId="factory-graph-add-cron-schedule"
        label={messages.cronScheduleFieldLabel}
        onChange={(value) => {
          onChange({ ...cron, schedule: value });
        }}
        value={cron.schedule}
      />
      <FactoryGraphEditorAddField
        inputId="factory-graph-add-cron-trigger-at-start"
        label={
          <>
            <Checkbox
              checked={cron.triggerAtStart}
              className="mr-2"
              id="factory-graph-add-cron-trigger-at-start"
              onChange={(event) => {
                onChange({
                  ...cron,
                  triggerAtStart: event.currentTarget.checked,
                });
              }}
            />
            {messages.cronTriggerAtStartFieldLabel}
          </>
        }
      >
        {null}
      </FactoryGraphEditorAddField>
      <FactoryGraphEditorTextField
        error={errors.cronJitter}
        helpText={messages.cronJitterFieldHint}
        inputId="factory-graph-add-cron-jitter"
        label={messages.cronJitterFieldLabel}
        onChange={(value) => {
          onChange({ ...cron, jitter: value });
        }}
        value={cron.jitter}
      />
      <FactoryGraphEditorTextField
        error={errors.cronExpiryWindow}
        helpText={messages.cronExpiryWindowFieldHint}
        inputId="factory-graph-add-cron-expiry-window"
        label={messages.cronExpiryWindowFieldLabel}
        onChange={(value) => {
          onChange({ ...cron, expiryWindow: value });
        }}
        value={cron.expiryWindow}
      />
    </>
  );
}
