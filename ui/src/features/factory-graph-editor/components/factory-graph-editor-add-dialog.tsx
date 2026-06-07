import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../../../components/ui";
import { EDITABLE_MODEL_PROVIDERS } from "../../current-factory-definition/lib/worker-editable-values";
import { getWorkerDetailMessages } from "../../current-selection/worker-selection/messages/worker-detail";
import type { CanonicalFactoryDefinition } from "../lib/factory-graph-draft-types";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
  FactoryGraphAddWorkerType,
} from "../lib/factory-graph-editor-additions";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import {
  FactoryGraphEditorSelectField,
  FactoryGraphEditorTextareaField,
  FactoryGraphEditorTextField,
} from "./factory-graph-editor-add-dialog-fields";
import { FactoryGraphEditorAddWorkerModelOperationsFields } from "./factory-graph-editor-add-worker-model-operations-fields";
import { FactoryGraphEditorAddWorkstationFields } from "./factory-graph-editor-add-workstation-fields";

export function FactoryGraphEditorAddEntityDialog({
  currentFactoryDefinition,
  draft,
  errors,
  isOpen,
  locale,
  onChange,
  onClose,
  onSubmit,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphAddEntityDraft | null;
  errors: FactoryGraphAddEntityFieldErrors;
  isOpen: boolean;
  locale?: string;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const isDialogOpen = isOpen && draft !== null;

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      onClose();
    }
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={isDialogOpen}>
      {draft ? (
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{messages.addDialogTitle(draft.kind)}</DialogTitle>
            <DialogDescription>
              {messages.addDialogDescription(draft.kind)}
            </DialogDescription>
          </DialogHeader>
          <FactoryGraphEditorAddEntityFields
            currentFactoryDefinition={currentFactoryDefinition}
            draft={draft}
            errors={errors}
            locale={locale}
            onChange={onChange}
            onSubmit={onSubmit}
          />
          <DialogFooter>
            <Button onClick={onClose} tone="outline" type="button">
              {messages.addDialogCancelAction}
            </Button>
            <Button onClick={onSubmit} type="button">
              {messages.addDialogAddEntityAction}
            </Button>
          </DialogFooter>
        </DialogContent>
      ) : null}
    </Dialog>
  );
}

function FactoryGraphEditorAddEntityFields({
  currentFactoryDefinition,
  draft,
  errors,
  locale,
  onChange,
  onSubmit,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphAddEntityDraft;
  errors: FactoryGraphAddEntityFieldErrors;
  locale?: string;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
  onSubmit: () => void;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  return (
    <form
      className="grid gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <FactoryGraphEditorTextField
        error={errors.name}
        helpText={messages.addDialogIdentifierHelp}
        inputId="factory-graph-add-name"
        label={messages.addDialogIdentifierLabel}
        onChange={(value) => {
          onChange({ ...draft, name: value });
        }}
        value={draft.name}
      />

      {renderEntitySpecificFields({
        currentFactoryDefinition,
        draft,
        errors,
        locale,
        onChange,
      })}
    </form>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: entity-specific add fields stay colocated in one renderer switch.
function renderEntitySpecificFields({
  currentFactoryDefinition,
  draft,
  errors,
  locale,
  onChange,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphAddEntityDraft;
  errors: FactoryGraphAddEntityFieldErrors;
  locale?: string;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  if (draft.kind === "resource") {
    return (
      <FactoryGraphEditorTextField
        error={errors.capacity}
        inputId="factory-graph-add-capacity"
        inputMode="numeric"
        label={messages.addDialogCapacityLabel}
        onChange={(value) => {
          onChange({ ...draft, capacity: value });
        }}
        value={draft.capacity}
      />
    );
  }

  if (draft.kind === "worker") {
    const workerMessages = getWorkerDetailMessages(locale);
    const isModelWorker = draft.workerType === "MODEL_WORKER";
    const isScriptWorker = draft.workerType === "SCRIPT_WORKER";

    return (
      <>
        <FactoryGraphEditorSelectField
          inputId="factory-graph-add-worker-type"
          label={workerMessages.typeFieldLabel}
          onChange={(value) => {
            const workerType = value as FactoryGraphAddWorkerType;
            if (workerType === draft.workerType) {
              return;
            }

            if (workerType === "MODEL_WORKER") {
              onChange({
                ...draft,
                argsText: "",
                command: "",
                operations: [],
                workerType,
              });
              return;
            }

            onChange({
              ...draft,
              model: "",
              modelProvider: "",
              operations: [],
              workerType,
            });
          }}
          options={(
            [
              "MODEL_WORKER",
              "SCRIPT_WORKER",
            ] as const satisfies FactoryGraphAddWorkerType[]
          ).map((workerType) => ({
            label: workerMessages.localizeWorkerType(workerType),
            value: workerType,
          }))}
          value={draft.workerType}
        />
        {isModelWorker ? (
          <>
            <FactoryGraphEditorSelectField
              error={errors.modelProvider}
              helpText={workerMessages.modelProviderFieldHelp}
              inputId="factory-graph-add-model-provider"
              label={workerMessages.modelProviderLabel}
              onChange={(value) => {
                onChange({ ...draft, modelProvider: value });
              }}
              options={[
                {
                  label: workerMessages.notConfiguredOptionLabel,
                  value: "",
                },
                ...EDITABLE_MODEL_PROVIDERS.map((provider) => ({
                  label: workerMessages.localizeModelProvider(provider),
                  value: provider,
                })),
              ]}
              value={draft.modelProvider}
            />
            <FactoryGraphEditorTextField
              error={errors.model}
              helpText={workerMessages.modelFieldHelp}
              inputId="factory-graph-add-model"
              label={workerMessages.modelLabel}
              onChange={(value) => {
                onChange({ ...draft, model: value });
              }}
              value={draft.model}
            />
            <FactoryGraphEditorAddWorkerModelOperationsFields
              errors={errors.modelOperations}
              locale={locale}
              onChange={(operations) => {
                onChange({ ...draft, operations });
              }}
              operations={draft.operations}
            />
          </>
        ) : null}
        {isScriptWorker ? (
          <>
            <FactoryGraphEditorTextField
              error={errors.command}
              inputId="factory-graph-add-command"
              label={workerMessages.commandFieldLabel}
              onChange={(value) => {
                onChange({ ...draft, command: value });
              }}
              value={draft.command}
            />
            <FactoryGraphEditorTextareaField
              error={errors.args}
              inputId="factory-graph-add-args"
              label={workerMessages.argsFieldLabel}
              onChange={(value) => {
                onChange({ ...draft, argsText: value });
              }}
              value={draft.argsText}
            />
          </>
        ) : null}
      </>
    );
  }

  if (draft.kind === "work-type") {
    return (
      <FactoryGraphEditorTextField
        error={errors.initialStateName}
        helpText={messages.addDialogFirstStateHelp}
        inputId="factory-graph-add-initial-state"
        label={messages.addDialogFirstStateLabel}
        onChange={(value) => {
          onChange({ ...draft, initialStateName: value });
        }}
        value={draft.initialStateName}
      />
    );
  }

  if (draft.kind === "work-state") {
    return (
      <>
        <FactoryGraphEditorSelectField
          error={errors.workTypeName}
          inputId="factory-graph-add-work-type"
          label={messages.addDialogWorkTypeLabel}
          onChange={(value) => {
            onChange({ ...draft, workTypeName: value });
          }}
          options={[
            { label: messages.addDialogWorkTypePlaceholder, value: "" },
            ...(currentFactoryDefinition?.workTypes ?? []).map((workType) => ({
              label: workType.name,
              value: workType.name,
            })),
          ]}
          value={draft.workTypeName}
        />
        <FactoryGraphEditorSelectField
          inputId="factory-graph-add-state-type"
          label={messages.addDialogStateTypeLabel}
          onChange={(value) => {
            onChange({
              ...draft,
              stateType: value as typeof draft.stateType,
            });
          }}
          options={[
            { label: messages.stateTypeLabel("INITIAL"), value: "INITIAL" },
            {
              label: messages.stateTypeLabel("PROCESSING"),
              value: "PROCESSING",
            },
            { label: messages.stateTypeLabel("TERMINAL"), value: "TERMINAL" },
            { label: messages.stateTypeLabel("FAILED"), value: "FAILED" },
          ]}
          value={draft.stateType}
        />
      </>
    );
  }

  return (
    <FactoryGraphEditorAddWorkstationFields
      currentFactoryDefinition={currentFactoryDefinition}
      draft={draft}
      errors={errors}
      locale={locale}
      messages={messages}
      onChange={onChange}
    />
  );
}
