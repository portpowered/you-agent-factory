import { Button, Checkbox } from "../../../components/ui";
import {
  createEmptyFactoryGraphAddModelOperationDraft,
  createEmptyFactoryGraphAddModelOperationSlotDraft,
  FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES,
  type FactoryGraphAddModelOperationDraft,
  type FactoryGraphAddModelOperationItemFieldErrors,
  type FactoryGraphAddModelOperationSlotDraft,
  type FactoryGraphAddModelOperationValidationErrors,
} from "../lib/factory-graph-add-model-operation-draft";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import {
  FactoryGraphEditorAddField,
  FactoryGraphEditorTextField,
} from "./add-dialog/factory-graph-editor-add-dialog-fields";

export function FactoryGraphEditorAddWorkerModelOperationsFields({
  errors,
  locale,
  onChange,
  operations,
}: {
  errors?: FactoryGraphAddModelOperationValidationErrors;
  locale?: string;
  onChange: (operations: FactoryGraphAddModelOperationDraft[]) => void;
  operations: FactoryGraphAddModelOperationDraft[];
}) {
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <FactoryGraphEditorAddField
      error={errors?.summary}
      helpText={messages.addDialogModelOperationsHelp}
      label={messages.addDialogModelOperationsLabel}
    >
      <div className="grid gap-4">
        {operations.map((operation, operationIndex) => (
          <section
            className="grid gap-3 rounded-md border border-outline p-3"
            // biome-ignore lint/suspicious/noArrayIndexKey: draft operations have no durable id before save.
            key={`model-operation-${operationIndex}`}
          >
            <div className="flex items-center justify-between gap-2">
              <h3 className="m-0 text-sm font-medium">
                {messages.addDialogModelOperationHeading(operationIndex)}
              </h3>
              <Button
                onClick={() => {
                  onChange(
                    operations.filter((_, index) => index !== operationIndex),
                  );
                }}
                size="sm"
                tone="outline"
                type="button"
              >
                {messages.addDialogModelOperationRemoveAction}
              </Button>
            </div>
            <FactoryGraphEditorTextField
              error={errors?.byIndex?.[operationIndex]?.name}
              helpText={messages.addDialogModelOperationNameHelp}
              inputId={`factory-graph-add-model-operation-name-${operationIndex}`}
              label={messages.addDialogModelOperationNameLabel}
              onChange={(value) => {
                onChange(
                  updateOperationAtIndex(operations, operationIndex, {
                    ...operation,
                    name: value,
                  }),
                );
              }}
              value={operation.name}
            />
            <ModelOperationSlotList
              direction="input"
              errors={errors?.byIndex?.[operationIndex]}
              locale={locale}
              onChange={(inputs) => {
                onChange(
                  updateOperationAtIndex(operations, operationIndex, {
                    ...operation,
                    inputs,
                  }),
                );
              }}
              slots={operation.inputs}
            />
            <ModelOperationSlotList
              direction="output"
              errors={errors?.byIndex?.[operationIndex]}
              locale={locale}
              onChange={(outputs) => {
                onChange(
                  updateOperationAtIndex(operations, operationIndex, {
                    ...operation,
                    outputs,
                  }),
                );
              }}
              slots={operation.outputs}
            />
          </section>
        ))}
        <Button
          onClick={() => {
            onChange([
              ...operations,
              createEmptyFactoryGraphAddModelOperationDraft(),
            ]);
          }}
          size="sm"
          tone="outline"
          type="button"
        >
          {messages.addDialogModelOperationAddAction}
        </Button>
      </div>
    </FactoryGraphEditorAddField>
  );
}

function ModelOperationSlotList({
  direction,
  errors,
  locale,
  onChange,
  slots,
}: {
  direction: "input" | "output";
  errors?: FactoryGraphAddModelOperationItemFieldErrors;
  locale?: string;
  onChange: (slots: FactoryGraphAddModelOperationSlotDraft[]) => void;
  slots: FactoryGraphAddModelOperationSlotDraft[];
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const directionLabel =
    direction === "input"
      ? messages.addDialogModelOperationInputsLabel
      : messages.addDialogModelOperationOutputsLabel;
  const directionError =
    direction === "input" ? errors?.inputs : errors?.outputs;
  const slotErrors =
    direction === "input" ? errors?.inputSlots : errors?.outputSlots;

  return (
    <FactoryGraphEditorAddField error={directionError} label={directionLabel}>
      <div className="grid gap-3">
        {slots.map((slot, slotIndex) => (
          <div
            className="grid gap-2 rounded-md border border-outline-variant p-2"
            // biome-ignore lint/suspicious/noArrayIndexKey: draft slots have no durable id before save.
            key={`${direction}-slot-${slotIndex}`}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-medium text-muted">
                {messages.addDialogModelOperationSlotHeading(
                  direction,
                  slotIndex,
                )}
              </span>
              <Button
                disabled={slots.length <= 1}
                onClick={() => {
                  onChange(slots.filter((_, index) => index !== slotIndex));
                }}
                size="sm"
                tone="outline"
                type="button"
              >
                {messages.addDialogModelOperationSlotRemoveAction}
              </Button>
            </div>
            <FactoryGraphEditorTextField
              error={slotErrors?.[slotIndex]?.name}
              inputId={`factory-graph-add-model-operation-${direction}-slot-name-${slotIndex}`}
              label={messages.addDialogModelOperationSlotNameLabel}
              onChange={(value) => {
                onChange(
                  updateSlotAtIndex(slots, slotIndex, {
                    ...slot,
                    name: value,
                  }),
                );
              }}
              value={slot.name}
            />
            <FactoryGraphEditorAddField
              error={slotErrors?.[slotIndex]?.contentTypes}
              label={messages.addDialogModelOperationSlotContentTypesLabel}
            >
              <div className="grid gap-2 sm:grid-cols-2">
                {FACTORY_GRAPH_ADD_MODEL_OPERATION_CONTENT_TYPES.map(
                  (contentType) => {
                    const checkboxId = `factory-graph-add-model-operation-${direction}-slot-${slotIndex}-content-type-${contentType}`;
                    const checked = slot.contentTypes.includes(contentType);

                    return (
                      <label
                        className="flex items-center gap-2 text-sm"
                        htmlFor={checkboxId}
                        key={checkboxId}
                      >
                        <Checkbox
                          checked={checked}
                          id={checkboxId}
                          onChange={(event) => {
                            const nextContentTypes = event.target.checked
                              ? [...slot.contentTypes, contentType]
                              : slot.contentTypes.filter(
                                  (value) => value !== contentType,
                                );
                            onChange(
                              updateSlotAtIndex(slots, slotIndex, {
                                ...slot,
                                contentTypes: nextContentTypes,
                              }),
                            );
                          }}
                        />
                        <span>
                          {messages.localizeModelOperationContentType(
                            contentType,
                          )}
                        </span>
                      </label>
                    );
                  },
                )}
              </div>
            </FactoryGraphEditorAddField>
            {direction === "input" ? (
              <label
                className="flex items-center gap-2 text-sm"
                htmlFor={`factory-graph-add-model-operation-input-slot-required-${slotIndex}`}
              >
                <Checkbox
                  checked={slot.required}
                  id={`factory-graph-add-model-operation-input-slot-required-${slotIndex}`}
                  onChange={(event) => {
                    onChange(
                      updateSlotAtIndex(slots, slotIndex, {
                        ...slot,
                        required: event.target.checked,
                      }),
                    );
                  }}
                />
                <span>{messages.addDialogModelOperationSlotRequiredLabel}</span>
              </label>
            ) : null}
          </div>
        ))}
        <Button
          onClick={() => {
            onChange([
              ...slots,
              createEmptyFactoryGraphAddModelOperationSlotDraft(direction),
            ]);
          }}
          size="sm"
          tone="outline"
          type="button"
        >
          {messages.addDialogModelOperationSlotAddAction(direction)}
        </Button>
      </div>
    </FactoryGraphEditorAddField>
  );
}

function updateOperationAtIndex(
  operations: FactoryGraphAddModelOperationDraft[],
  operationIndex: number,
  nextOperation: FactoryGraphAddModelOperationDraft,
) {
  return operations.map((operation, index) =>
    index === operationIndex ? nextOperation : operation,
  );
}

function updateSlotAtIndex(
  slots: FactoryGraphAddModelOperationSlotDraft[],
  slotIndex: number,
  nextSlot: FactoryGraphAddModelOperationSlotDraft,
) {
  return slots.map((slot, index) => (index === slotIndex ? nextSlot : slot));
}
