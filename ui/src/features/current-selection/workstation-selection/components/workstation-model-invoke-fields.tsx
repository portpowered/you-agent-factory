import {
  EnumSelect,
  OptionalEnumSelect,
} from "@you-agent-factory/components/forms";
import type { ReactNode } from "react";
import type { ModelOperationContentType } from "../../../../api/generated/openapi";
import {
  FormDescription,
  FormError,
  Input,
  Label,
} from "../../../../components/ui";
import { FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES } from "../../../factory-graph-editor/lib/factory-graph-add-model-operation-draft";
import { updateEditableModelInvokeBindingDraft } from "../editing/model-invoke/editable-workstation-model-invoke-mutators";
import { resolveModelInvokeBindingInputSlots } from "../lib/editable-workstation-model-invoke-options";
import type { WorkstationDetailCardProps } from "../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

type ReadyEditableConfigurationState = Extract<
  NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
  { status: "ready" }
>;

export function EditableConfigurationModelInvokeFields({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
  validationErrors: ReadyEditableConfigurationState["validationErrors"];
}) {
  return (
    <>
      <EditableConfigurationField
        errorMessage={validationErrors.workerName}
        fieldId="editable-workstation-worker"
        input={
          <EditableConfigurationModelInvokeWorkerInput
            messages={messages}
            state={state}
          />
        }
        label={messages.workerFieldLabel}
      />
      <EditableConfigurationField
        errorMessage={validationErrors.operation}
        fieldId="editable-workstation-operation"
        input={
          <EditableConfigurationModelInvokeOperationInput
            messages={messages}
            state={state}
          />
        }
        label={messages.modelInvokeOperationFieldLabel}
      />
      <EditableConfigurationModelInvokeBindingsField
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    </>
  );
}

function EditableConfigurationModelInvokeWorkerInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
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
        label: workerName,
        value: workerName,
      }))}
      value={state.draft.workerName}
    />
  );
}

function EditableConfigurationModelInvokeOperationInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
}) {
  if (state.operationOptionsState.status === "empty") {
    return (
      <FormDescription variant="body">
        {state.operationOptionsState.message}
      </FormDescription>
    );
  }

  if (state.operationOptionsState.status === "error") {
    return <FormError>{state.operationOptionsState.message}</FormError>;
  }

  return (
    <EnumSelect
      aria-describedby={
        state.validationErrors.operation
          ? "editable-workstation-operation-error"
          : undefined
      }
      aria-invalid={state.validationErrors.operation ? "true" : undefined}
      aria-label={messages.modelInvokeOperationFieldLabel}
      id="editable-workstation-operation"
      onValueChange={state.onOperationChange}
      options={state.operationOptionsState.options.map((operationName) => ({
        label: operationName,
        value: operationName,
      }))}
      value={state.draft.operation}
    />
  );
}

function EditableConfigurationModelInvokeBindingsField({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
  validationErrors: ReadyEditableConfigurationState["validationErrors"];
}) {
  const inputSlots = resolveModelInvokeBindingInputSlots(
    state.draft,
    state.initialValues,
  );

  if (inputSlots.length === 0) {
    return (
      <EditableConfigurationField
        fieldId="editable-workstation-operation-bindings"
        input={
          <FormDescription variant="body">
            {messages.modelInvokeBindingsEmpty}
          </FormDescription>
        }
        label={messages.modelInvokeBindingsFieldLabel}
      />
    );
  }

  return (
    <EditableConfigurationField
      errorMessage={validationErrors.operationBindings}
      fieldId="editable-workstation-operation-bindings"
      input={
        <div className="grid gap-3">
          {inputSlots.map((inputSlot) => (
            <EditableConfigurationModelInvokeBindingSlotCard
              inputSlot={inputSlot}
              key={inputSlot.name}
              messages={messages}
              state={state}
              validationError={
                validationErrors[`operationBindings[${inputSlot.name}]`]
              }
            />
          ))}
        </div>
      }
      label={messages.modelInvokeBindingsFieldLabel}
      supportingContent={
        <FormDescription variant="body">
          {messages.modelInvokeBindingsFieldHint}
        </FormDescription>
      }
    />
  );
}

function EditableConfigurationModelInvokeBindingSlotCard({
  inputSlot,
  messages,
  state,
  validationError,
}: {
  inputSlot: {
    name: string;
    required?: boolean;
  };
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
  validationError?: string;
}) {
  const binding =
    state.draft.operationBindings.find(
      (entry) => entry.slot === inputSlot.name,
    ) ?? null;
  const selector = binding?.selector ?? {
    label: "",
    role: "",
    slot: "",
    type: "",
  };

  return (
    <div className="grid gap-2 rounded-xl border border-outline-variant p-3">
      <Label as="p">
        {messages.modelInvokeBindingSlotHeading(
          inputSlot.name,
          inputSlot.required
            ? messages.modelInvokeBindingRequiredSlotLabel
            : messages.modelInvokeBindingOptionalSlotLabel,
        )}
      </Label>
      <div className="grid gap-2 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
        <BindingSelectorField
          fieldId={`editable-workstation-binding-${inputSlot.name}-label`}
          label={messages.modelInvokeBindingSelectorLabelFieldLabel}
          onChange={(value) =>
            updateBindingSelector(state, inputSlot.name, "label", value)
          }
          value={selector.label}
        />
        <BindingSelectorField
          fieldId={`editable-workstation-binding-${inputSlot.name}-slot`}
          label={messages.modelInvokeBindingSelectorSlotFieldLabel}
          onChange={(value) =>
            updateBindingSelector(state, inputSlot.name, "slot", value)
          }
          value={selector.slot}
        />
        <BindingSelectorField
          fieldId={`editable-workstation-binding-${inputSlot.name}-role`}
          label={messages.modelInvokeBindingSelectorRoleFieldLabel}
          onChange={(value) =>
            updateBindingSelector(state, inputSlot.name, "role", value)
          }
          value={selector.role}
        />
        <div className="grid gap-1">
          <Label
            as="label"
            htmlFor={`editable-workstation-binding-${inputSlot.name}-type`}
          >
            {messages.modelInvokeBindingSelectorTypeFieldLabel}
          </Label>
          <OptionalEnumSelect
            aria-label={messages.modelInvokeBindingSelectorTypeFieldLabel}
            emptyOptionLabel={messages.modelInvokeBindingSelectorTypeNoneOption}
            id={`editable-workstation-binding-${inputSlot.name}-type`}
            onValueChange={(nextValue) =>
              updateBindingSelector(
                state,
                inputSlot.name,
                "type",
                nextValue ?? "",
              )
            }
            options={FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES.map(
              (contentType) => ({
                label: contentType,
                value: contentType,
              }),
            )}
            value={selector.type || null}
          />
        </div>
        <BindingSelectorField
          fieldId={`editable-workstation-binding-${inputSlot.name}-config`}
          label={messages.modelInvokeBindingConfigContentFieldLabel}
          onChange={(value) =>
            updateBindingContent(state, inputSlot.name, "configText", value)
          }
          value={binding?.configText ?? ""}
        />
        <BindingSelectorField
          fieldId={`editable-workstation-binding-${inputSlot.name}-default`}
          label={messages.modelInvokeBindingDefaultContentFieldLabel}
          onChange={(value) =>
            updateBindingContent(
              state,
              inputSlot.name,
              "defaultContentText",
              value,
            )
          }
          value={binding?.defaultContentText ?? ""}
        />
      </div>
      {validationError ? <FormError>{validationError}</FormError> : null}
    </div>
  );
}

function updateBindingSelector(
  state: ReadyEditableConfigurationState,
  slot: string,
  field: "label" | "role" | "slot" | "type",
  value: string,
) {
  const existingBindings = state.draft.operationBindings;
  const bindings =
    existingBindings.find((binding) => binding.slot === slot) != null
      ? existingBindings
      : [
          ...existingBindings,
          {
            slot,
            configText: "",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" as const },
          },
        ];

  state.onOperationBindingsChange(
    updateEditableModelInvokeBindingDraft(bindings, slot, (binding) => ({
      ...binding,
      selector: {
        ...binding.selector,
        [field]:
          field === "type"
            ? (value as
                | (typeof ModelOperationContentType)[keyof typeof ModelOperationContentType]
                | "")
            : value,
      },
    })),
  );
}

function updateBindingContent(
  state: ReadyEditableConfigurationState,
  slot: string,
  field: "configText" | "defaultContentText",
  value: string,
) {
  const existingBindings = state.draft.operationBindings;
  const bindings =
    existingBindings.find((binding) => binding.slot === slot) != null
      ? existingBindings
      : [
          ...existingBindings,
          {
            slot,
            configText: "",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" as const },
          },
        ];

  state.onOperationBindingsChange(
    updateEditableModelInvokeBindingDraft(bindings, slot, (binding) => ({
      ...binding,
      [field]: value,
    })),
  );
}

function BindingSelectorField({
  fieldId,
  label,
  onChange,
  value,
}: {
  fieldId: string;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <div className="grid gap-1">
      <Label as="label" htmlFor={fieldId}>
        {label}
      </Label>
      <Input
        id={fieldId}
        onChange={(event) => onChange(event.target.value)}
        type="text"
        value={value}
      />
    </div>
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
  supportingContent?: React.ReactNode;
}) {
  return (
    <div className="grid gap-1">
      <Label as="label" htmlFor={fieldId}>
        {label}
      </Label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <FormError id={`${fieldId}-error`}>{errorMessage}</FormError>
      ) : null}
    </div>
  );
}
