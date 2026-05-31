import { MonacoPromptEditor } from "../../../components/prompt-editor";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Select,
} from "../../../components/ui";
import type { CanonicalFactoryDefinition } from "../lib/factory-graph-draft-types";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
} from "../lib/factory-graph-editor-additions";
import { editableWorkstationBehaviorOptions } from "../lib/factory-graph-editor-additions";
import { getFactoryGraphEditorMessages } from "../messages/editor";

const FIELD_GROUP_CLASS = "grid gap-2";
const FIELD_LABEL_CLASS = "text-sm font-semibold text-af-text";
const FIELD_HELP_CLASS = "m-0 text-xs leading-5 text-af-text-muted";
const FIELD_ERROR_CLASS = "m-0 text-sm text-af-danger-text";
const INPUT_CLASS = "bg-af-surface";

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
    return (
      <FactoryGraphEditorTextField
        error={errors.model}
        helpText={messages.addDialogModelHelp}
        inputId="factory-graph-add-model"
        label={messages.addDialogModelLabel}
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
    <>
      <FactoryGraphEditorSelectField
        error={errors.behavior}
        inputId="factory-graph-add-workstation-kind"
        label={messages.addDialogKindLabel}
        onChange={(value) => {
          onChange({
            ...draft,
            behavior: value as typeof draft.behavior,
          });
        }}
        options={editableWorkstationBehaviorOptions().map((behavior) => ({
          label: behavior,
          value: behavior,
        }))}
        value={draft.behavior}
      />
      <FactoryGraphEditorSelectField
        error={errors.workerName}
        inputId="factory-graph-add-worker-name"
        label={messages.addDialogAssignedWorkerLabel}
        onChange={(value) => {
          onChange({ ...draft, workerName: value });
        }}
        options={[
          { label: messages.addDialogAssignedWorkerPlaceholder, value: "" },
          ...(currentFactoryDefinition?.workers ?? []).map((worker) => ({
            label: worker.name,
            value: worker.name,
          })),
        ]}
        value={draft.workerName}
      />
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

const factoryGraphAddPromptAutocompleteState = {
  message: "",
  status: "empty" as const,
};

function FactoryGraphEditorPromptBodyField({
  helpText,
  label,
  loadingMessage,
  onChange,
  startupErrorMessage,
  value,
}: {
  helpText?: string;
  label: string;
  loadingMessage: string;
  onChange: (value: string) => void;
  startupErrorMessage: string;
  value: string;
}) {
  return (
    <div className={FIELD_GROUP_CLASS}>
      <p className={FIELD_LABEL_CLASS}>{label}</p>
      <MonacoPromptEditor
        ariaLabel={label}
        autocompleteState={factoryGraphAddPromptAutocompleteState}
        className={INPUT_CLASS}
        loadingMessage={loadingMessage}
        onChange={onChange}
        startupErrorMessage={startupErrorMessage}
        value={value}
      />
      {helpText ? <p className={FIELD_HELP_CLASS}>{helpText}</p> : null}
    </div>
  );
}
