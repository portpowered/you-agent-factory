import { Button, Input, Select, Textarea } from "../../components/ui";
import { DashboardMutationDialog } from "../workflow-activity/mutation-dialog";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
} from "./factory-graph-editor-additions";

const FIELD_GROUP_CLASS = "grid gap-2";
const FIELD_LABEL_CLASS = "text-sm font-semibold text-af-ink";
const FIELD_HELP_CLASS = "m-0 text-xs leading-5 text-af-ink/66";
const FIELD_ERROR_CLASS = "m-0 text-sm text-af-danger-ink";
const INPUT_CLASS = "bg-af-canvas";

export function FactoryGraphEditorAddEntityDialog({
  currentFactoryDefinition,
  draft,
  errors,
  isOpen,
  onChange,
  onClose,
  onSubmit,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphAddEntityDraft | null;
  errors: FactoryGraphAddEntityFieldErrors;
  isOpen: boolean;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
  onClose: () => void;
  onSubmit: () => void;
}) {
  if (!isOpen || draft === null) {
    return null;
  }

  return (
    <DashboardMutationDialog
      description={dialogDescription(draft.kind)}
      onClose={onClose}
      title={dialogTitle(draft.kind)}
      footer={
        <>
          <Button onClick={onClose} tone="outline" type="button">
            Cancel
          </Button>
          <Button onClick={onSubmit} type="button">
            Add entity
          </Button>
        </>
      }
    >
      <FactoryGraphEditorAddEntityFields
        currentFactoryDefinition={currentFactoryDefinition}
        draft={draft}
        errors={errors}
        onChange={onChange}
        onSubmit={onSubmit}
      />
    </DashboardMutationDialog>
  );
}

function FactoryGraphEditorAddEntityFields({
  currentFactoryDefinition,
  draft,
  errors,
  onChange,
  onSubmit,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphAddEntityDraft;
  errors: FactoryGraphAddEntityFieldErrors;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
  onSubmit: () => void;
}) {
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
        helpText="Use the authored name the factory definition should save."
        inputId="factory-graph-add-name"
        label="Identifier"
        onChange={(value) => {
          onChange({ ...draft, name: value });
        }}
        value={draft.name}
      />

      {renderEntitySpecificFields({
        currentFactoryDefinition,
        draft,
        errors,
        onChange,
      })}
    </form>
  );
}

function dialogTitle(kind: FactoryGraphAddEntityDraft["kind"]) {
  if (kind === "work-type") {
    return "Add work type";
  }
  if (kind === "work-state") {
    return "Add work state";
  }
  return `Add ${kind}`;
}

function dialogDescription(kind: FactoryGraphAddEntityDraft["kind"]) {
  if (kind === "workstation") {
    return "Create a pending workstation in the current graph draft.";
  }
  if (kind === "work-type") {
    return "Define a new work type and its first ordered state.";
  }
  if (kind === "work-state") {
    return "Append a new ordered state to an existing work type.";
  }
  return `Create a pending ${kind} in the current graph draft.`;
}

function renderEntitySpecificFields({
  currentFactoryDefinition,
  draft,
  errors,
  onChange,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphAddEntityDraft;
  errors: FactoryGraphAddEntityFieldErrors;
  onChange: (draft: FactoryGraphAddEntityDraft) => void;
}) {
  if (draft.kind === "resource") {
    return (
      <FactoryGraphEditorTextField
        error={errors.capacity}
        inputId="factory-graph-add-capacity"
        inputMode="numeric"
        label="Capacity"
        onChange={(value) => {
          onChange({ ...draft, capacity: value });
        }}
        value={draft.capacity}
      />
    );
  }

  if (draft.kind === "worker") {
    return (
      <FactoryGraphEditorTextField
        error={errors.model}
        helpText="The model identifier saved on the new `MODEL_WORKER`."
        inputId="factory-graph-add-model"
        label="Model"
        onChange={(value) => {
          onChange({ ...draft, model: value });
        }}
        value={draft.model}
      />
    );
  }

  if (draft.kind === "work-type") {
    return (
      <FactoryGraphEditorTextField
        error={errors.initialStateName}
        helpText="New work types start with one required ordered state."
        inputId="factory-graph-add-initial-state"
        label="First state"
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
          label="Work type"
          onChange={(value) => {
            onChange({ ...draft, workTypeName: value });
          }}
          options={[
            { label: "Select a work type", value: "" },
            ...(currentFactoryDefinition?.workTypes ?? []).map((workType) => ({
              label: workType.name,
              value: workType.name,
            })),
          ]}
          value={draft.workTypeName}
        />
        <FactoryGraphEditorSelectField
          inputId="factory-graph-add-state-type"
          label="State type"
          onChange={(value) => {
            onChange({
              ...draft,
              stateType: value as typeof draft.stateType,
            });
          }}
          options={[
            { label: "INITIAL", value: "INITIAL" },
            { label: "PROCESSING", value: "PROCESSING" },
            { label: "TERMINAL", value: "TERMINAL" },
            { label: "FAILED", value: "FAILED" },
          ]}
          value={draft.stateType}
        />
      </>
    );
  }

  return (
    <>
      <FactoryGraphEditorSelectField
        error={errors.workerName}
        inputId="factory-graph-add-worker-name"
        label="Assigned worker"
        onChange={(value) => {
          onChange({ ...draft, workerName: value });
        }}
        options={[
          { label: "Select a worker", value: "" },
          ...(currentFactoryDefinition?.workers ?? []).map((worker) => ({
            label: worker.name,
            value: worker.name,
          })),
        ]}
        value={draft.workerName}
      />
      <FactoryGraphEditorTextareaField
        helpText="Optional prompt content for the workstation body."
        inputId="factory-graph-add-workstation-body"
        label="Prompt body"
        onChange={(value) => {
          onChange({ ...draft, body: value });
        }}
        value={draft.body}
      />
    </>
  );
}

function FactoryGraphEditorTextField({
  error,
  helpText,
  inputId,
  inputMode,
  label,
  onChange,
  value,
}: {
  error?: string;
  helpText?: string;
  inputId: string;
  inputMode?: "numeric";
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <div className={FIELD_GROUP_CLASS}>
      <label className={FIELD_LABEL_CLASS} htmlFor={inputId}>
        {label}
      </label>
      <Input
        aria-label={label}
        className={INPUT_CLASS}
        id={inputId}
        inputMode={inputMode}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        value={value}
      />
      {helpText ? <p className={FIELD_HELP_CLASS}>{helpText}</p> : null}
      {error ? <p className={FIELD_ERROR_CLASS}>{error}</p> : null}
    </div>
  );
}

function FactoryGraphEditorSelectField({
  error,
  inputId,
  label,
  onChange,
  options,
  value,
}: {
  error?: string;
  inputId: string;
  label: string;
  onChange: (value: string) => void;
  options: Array<{ label: string; value: string }>;
  value: string;
}) {
  return (
    <div className={FIELD_GROUP_CLASS}>
      <label className={FIELD_LABEL_CLASS} htmlFor={inputId}>
        {label}
      </label>
      <Select
        aria-label={label}
        className={INPUT_CLASS}
        id={inputId}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        value={value}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </Select>
      {error ? <p className={FIELD_ERROR_CLASS}>{error}</p> : null}
    </div>
  );
}

function FactoryGraphEditorTextareaField({
  helpText,
  inputId,
  label,
  onChange,
  value,
}: {
  helpText?: string;
  inputId: string;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <div className={FIELD_GROUP_CLASS}>
      <label className={FIELD_LABEL_CLASS} htmlFor={inputId}>
        {label}
      </label>
      <Textarea
        aria-label={label}
        className={INPUT_CLASS}
        id={inputId}
        onChange={(event) => {
          onChange(event.currentTarget.value);
        }}
        rows={5}
        value={value}
      />
      {helpText ? <p className={FIELD_HELP_CLASS}>{helpText}</p> : null}
    </div>
  );
}
