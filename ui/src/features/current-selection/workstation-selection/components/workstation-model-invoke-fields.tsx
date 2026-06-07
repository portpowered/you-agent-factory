import type { ReactNode } from "react";
import type { ModelOperationContentType } from "../../../../api/generated/openapi";
import {
  DashboardLabel,
  DashboardText,
  FormDescription,
  FormError,
  Input,
  Select,
} from "../../../../components/ui";
import { FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES } from "../../../factory-graph-editor/lib/factory-graph-add-model-operation-draft";
import { updateEditableModelInvokeBindingDraft } from "../editing/editable-workstation-model-invoke-mutators";
import type { WorkstationDetailCardProps } from "../lib/detail-card-types";
import { resolveModelInvokeBindingInputSlots } from "../lib/editable-workstation-model-invoke-options";
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
        fieldId="editable-workstation-type"
        input={
          <DashboardText className="m-0 text-sm text-on-surface" variant="body">
            {messages.localizeWorkstationType(
              state.initialValues.workstationType,
            )}
          </DashboardText>
        }
        label={messages.workstationTypeLabel}
      />
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
        input={<EditableConfigurationModelInvokeOperationInput state={state} />}
        label={messages.modelInvokeOperationFieldLabel}
      />
      <EditableConfigurationModelInvokeBindingsField
        messages={messages}
        state={state}
        validationError={validationErrors.operationBindings}
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
    <Select
      aria-describedby={
        state.validationErrors.workerName
          ? "editable-workstation-worker-error"
          : undefined
      }
      aria-invalid={state.validationErrors.workerName ? "true" : undefined}
      id="editable-workstation-worker"
      onChange={(event) => state.onWorkerChange(event.target.value)}
      value={state.draft.workerName}
    >
      {state.workerOptionsState.options.map((workerName) => (
        <option key={workerName} value={workerName}>
          {workerName}
        </option>
      ))}
    </Select>
  );
}

function EditableConfigurationModelInvokeOperationInput({
  state,
}: {
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
    <Select
      aria-describedby={
        state.validationErrors.operation
          ? "editable-workstation-operation-error"
          : undefined
      }
      aria-invalid={state.validationErrors.operation ? "true" : undefined}
      id="editable-workstation-operation"
      onChange={(event) => state.onOperationChange(event.target.value)}
      value={state.draft.operation}
    >
      {state.operationOptionsState.options.map((operationName) => (
        <option key={operationName} value={operationName}>
          {operationName}
        </option>
      ))}
    </Select>
  );
}

function EditableConfigurationModelInvokeBindingsField({
  messages,
  state,
  validationError,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
  validationError?: string;
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
      errorMessage={validationError}
      fieldId="editable-workstation-operation-bindings"
      input={
        <div className="grid gap-3">
          {inputSlots.map((inputSlot) => {
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
              <div
                className="grid gap-2 rounded-xl border border-outline-variant p-3"
                key={inputSlot.name}
              >
                <DashboardLabel as="p">
                  {messages.modelInvokeBindingSlotHeading(
                    inputSlot.name,
                    inputSlot.required
                      ? messages.modelInvokeBindingRequiredSlotLabel
                      : messages.modelInvokeBindingOptionalSlotLabel,
                  )}
                </DashboardLabel>
                <div className="grid gap-2 [grid-template-columns:repeat(auto-fit,minmax(8.75rem,1fr))]">
                  <BindingSelectorField
                    fieldId={`editable-workstation-binding-${inputSlot.name}-label`}
                    label={messages.modelInvokeBindingSelectorLabelFieldLabel}
                    onChange={(value) =>
                      updateBindingSelector(
                        state,
                        inputSlot.name,
                        "label",
                        value,
                      )
                    }
                    value={selector.label}
                  />
                  <BindingSelectorField
                    fieldId={`editable-workstation-binding-${inputSlot.name}-slot`}
                    label={messages.modelInvokeBindingSelectorSlotFieldLabel}
                    onChange={(value) =>
                      updateBindingSelector(
                        state,
                        inputSlot.name,
                        "slot",
                        value,
                      )
                    }
                    value={selector.slot}
                  />
                  <BindingSelectorField
                    fieldId={`editable-workstation-binding-${inputSlot.name}-role`}
                    label={messages.modelInvokeBindingSelectorRoleFieldLabel}
                    onChange={(value) =>
                      updateBindingSelector(
                        state,
                        inputSlot.name,
                        "role",
                        value,
                      )
                    }
                    value={selector.role}
                  />
                  <div className="grid gap-1">
                    <DashboardLabel
                      as="label"
                      htmlFor={`editable-workstation-binding-${inputSlot.name}-type`}
                    >
                      {messages.modelInvokeBindingSelectorTypeFieldLabel}
                    </DashboardLabel>
                    <Select
                      id={`editable-workstation-binding-${inputSlot.name}-type`}
                      onChange={(event) =>
                        updateBindingSelector(
                          state,
                          inputSlot.name,
                          "type",
                          event.target.value,
                        )
                      }
                      value={selector.type}
                    >
                      <option value="">
                        {messages.modelInvokeBindingSelectorTypeNoneOption}
                      </option>
                      {FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES.map(
                        (contentType) => (
                          <option key={contentType} value={contentType}>
                            {contentType}
                          </option>
                        ),
                      )}
                    </Select>
                  </div>
                </div>
              </div>
            );
          })}
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
      <DashboardLabel as="label" htmlFor={fieldId}>
        {label}
      </DashboardLabel>
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
      <DashboardLabel as="label" htmlFor={fieldId}>
        {label}
      </DashboardLabel>
      {input}
      {supportingContent}
      {errorMessage ? (
        <FormError id={`${fieldId}-error`}>{errorMessage}</FormError>
      ) : null}
    </div>
  );
}
