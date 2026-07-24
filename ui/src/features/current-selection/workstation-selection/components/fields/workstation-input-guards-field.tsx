import {
  EnumSelect,
  OptionalEnumSelect,
} from "@you-agent-factory/components/forms";
import { useId } from "react";

import {
  FormDescription,
  FormError,
  Input,
  Label,
  SurfacePanel,
  Text,
} from "../../../../../components/ui";
import type { EditableWorkstationInputDraft } from "../../../../current-factory-definition/lib/workstation-editable-values";
import {
  createDefaultInputGuard,
  formatInputGuardSummary,
  INPUT_GUARD_TYPES,
  type InputGuard,
  type InputGuardBase,
  type InputGuardType,
  resolvePeerInputWorkTypes,
  setEditableInputSlotGuard,
} from "../../../../current-factory-definition/lib/workstation-guards";
import { CurrentSelectionFormField } from "../../../base/public";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";

export function EditableConfigurationWorkstationInputGuardsField({
  fieldErrors = {},
  inputs,
  messages,
  onInputsChange,
}: {
  fieldErrors?: Record<string, string | undefined>;
  inputs: EditableWorkstationInputDraft[];
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onInputsChange: (inputs: EditableWorkstationInputDraft[]) => void;
}) {
  const sectionId = useId();

  if (inputs.length === 0) {
    return (
      <CurrentSelectionFormField>
        <Label as="h5" className="m-0">
          {messages.workstationInputGuardsHeading}
        </Label>
        <FormDescription variant="body">
          {messages.workstationInputGuardsEmpty}
        </FormDescription>
      </CurrentSelectionFormField>
    );
  }

  return (
    <CurrentSelectionFormField>
      <Label as="h5" className="m-0">
        {messages.workstationInputGuardsHeading}
      </Label>
      <ul className="m-0 grid list-none gap-2 p-0">
        {inputs.map((input, slotIndex) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: input slots have no stable server id until save
          <li key={`${input.workType}-${input.state}-${slotIndex}`}>
            <WorkstationInputSlotGuardRow
              fieldErrors={fieldErrors}
              input={input}
              messages={messages}
              onChange={(nextInput) => {
                onInputsChange(
                  inputs.map((entry, entryIndex) =>
                    entryIndex === slotIndex ? nextInput : entry,
                  ),
                );
              }}
              peerWorkTypes={resolvePeerInputWorkTypes(inputs, slotIndex)}
              rowId={`${sectionId}-input-${slotIndex}`}
              slotIndex={slotIndex}
            />
          </li>
        ))}
      </ul>
    </CurrentSelectionFormField>
  );
}

function WorkstationInputSlotGuardRow({
  fieldErrors,
  input,
  messages,
  onChange,
  peerWorkTypes,
  rowId,
  slotIndex,
}: {
  fieldErrors: Record<string, string | undefined>;
  input: EditableWorkstationInputDraft;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (input: EditableWorkstationInputDraft) => void;
  peerWorkTypes: string[];
  rowId: string;
  slotIndex: number;
}) {
  const guard = resolveEditableInputGuard(input.guards);
  const guardTypeFieldId = `${rowId}-guard-type`;
  const guardTypeValue = guard?.type ?? "";

  return (
    <SurfacePanel
      aria-labelledby={`${rowId}-heading`}
      asChild
      className="grid gap-2"
      radius="lg"
    >
      <article>
        <div className="grid min-w-0 gap-1">
          <h6 className="m-0 text-sm text-on-surface" id={`${rowId}-heading`}>
            {messages.workstationInputSlotHeading(input.workType, input.state)}
          </h6>
          {guard ? (
            <Text className="m-0 text-on-surface-subtle" variant="supporting">
              {messages.localizeInputGuardType(guard.type)} ·{" "}
              {formatInputGuardSummary(guard)}
            </Text>
          ) : null}
        </div>
        <div className="grid gap-2">
          <Label as="label" htmlFor={guardTypeFieldId}>
            {messages.workstationInputGuardTypeFieldLabel}
          </Label>
          <OptionalEnumSelect
            aria-label={messages.workstationInputGuardTypeFieldLabel}
            emptyOptionLabel={messages.workstationInputGuardNoneOption}
            id={guardTypeFieldId}
            onValueChange={(nextType) => {
              if (!nextType) {
                onChange(setEditableInputSlotGuard(input, null));
                return;
              }

              onChange(
                setEditableInputSlotGuard(
                  input,
                  createDefaultInputGuard(
                    nextType as InputGuardType,
                    peerWorkTypes,
                  ),
                ),
              );
            }}
            options={INPUT_GUARD_TYPES.map((guardType) => ({
              label: messages.localizeInputGuardType(guardType),
              value: guardType,
            }))}
            value={guardTypeValue || null}
          />
          {resolveFieldError(fieldErrors, slotIndex, "type") ? (
            <GuardFieldError
              message={resolveFieldError(fieldErrors, slotIndex, "type") ?? ""}
            />
          ) : null}
        </div>
        {guard?.type === "SAME_NAME" || guard?.type === "SAME_TRACE_ID" ? (
          <PeerInputGuardFields
            fieldErrors={fieldErrors}
            guard={guard}
            matchInputFieldLabel={messages.inputGuardMatchInputFieldLabel}
            messages={messages}
            onChange={(nextGuard) => {
              onChange(setEditableInputSlotGuard(input, nextGuard));
            }}
            peerWorkTypes={peerWorkTypes}
            rowId={rowId}
            slotIndex={slotIndex}
          />
        ) : null}
        {guard?.type === "ALL_CHILDREN_COMPLETE" ||
        guard?.type === "ANY_CHILD_FAILED" ? (
          <ParentInputGuardFields
            fieldErrors={fieldErrors}
            guard={guard}
            messages={messages}
            onChange={(nextGuard) => {
              onChange(setEditableInputSlotGuard(input, nextGuard));
            }}
            peerWorkTypes={peerWorkTypes}
            rowId={rowId}
            slotIndex={slotIndex}
          />
        ) : null}
      </article>
    </SurfacePanel>
  );
}

function PeerInputGuardFields({
  fieldErrors,
  guard,
  matchInputFieldLabel,
  messages,
  onChange,
  peerWorkTypes,
  rowId,
  slotIndex,
}: {
  fieldErrors: Record<string, string | undefined>;
  guard: Extract<InputGuard, { type: "SAME_NAME" | "SAME_TRACE_ID" }>;
  matchInputFieldLabel: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (guard: InputGuard) => void;
  peerWorkTypes: string[];
  rowId: string;
  slotIndex: number;
}) {
  const matchInputFieldId = `${rowId}-match-input`;

  return (
    <div className="grid gap-1">
      <Label as="label" htmlFor={matchInputFieldId}>
        {matchInputFieldLabel}
      </Label>
      {peerWorkTypes.length === 0 ? (
        <FormDescription variant="body">
          {messages.workstationInputGuardPeersEmpty}
        </FormDescription>
      ) : (
        <EnumSelect
          aria-label={matchInputFieldLabel}
          id={matchInputFieldId}
          onValueChange={(nextValue) => {
            onChange({
              ...guard,
              matchInput: nextValue,
            });
          }}
          options={peerWorkTypes.map((workType) => ({
            label: workType,
            value: workType,
          }))}
          value={guard.matchInput ?? peerWorkTypes[0] ?? ""}
        />
      )}
      {resolveFieldError(fieldErrors, slotIndex, "matchInput") ? (
        <GuardFieldError
          message={
            resolveFieldError(fieldErrors, slotIndex, "matchInput") ?? ""
          }
        />
      ) : null}
    </div>
  );
}

function ParentInputGuardFields({
  fieldErrors,
  guard,
  messages,
  onChange,
  peerWorkTypes,
  rowId,
  slotIndex,
}: {
  fieldErrors: Record<string, string | undefined>;
  guard: Extract<
    InputGuard,
    { type: "ALL_CHILDREN_COMPLETE" | "ANY_CHILD_FAILED" }
  >;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (guard: InputGuard) => void;
  peerWorkTypes: string[];
  rowId: string;
  slotIndex: number;
}) {
  const parentInputFieldId = `${rowId}-parent-input`;
  const spawnedByFieldId = `${rowId}-spawned-by`;

  return (
    <div className="grid gap-2 md:grid-cols-2">
      <div className="grid gap-1">
        <Label as="label" htmlFor={parentInputFieldId}>
          {messages.inputGuardParentInputFieldLabel}
        </Label>
        {peerWorkTypes.length === 0 ? (
          <FormDescription variant="body">
            {messages.workstationInputGuardPeersEmpty}
          </FormDescription>
        ) : (
          <EnumSelect
            aria-label={messages.inputGuardParentInputFieldLabel}
            id={parentInputFieldId}
            onValueChange={(nextValue) => {
              onChange({
                ...guard,
                parentInput: nextValue,
              });
            }}
            options={peerWorkTypes.map((workType) => ({
              label: workType,
              value: workType,
            }))}
            value={guard.parentInput ?? peerWorkTypes[0] ?? ""}
          />
        )}
        {resolveFieldError(fieldErrors, slotIndex, "parentInput") ? (
          <GuardFieldError
            message={
              resolveFieldError(fieldErrors, slotIndex, "parentInput") ?? ""
            }
          />
        ) : null}
      </div>
      <div className="grid gap-1">
        <Label as="label" htmlFor={spawnedByFieldId}>
          {messages.inputGuardSpawnedByFieldLabel}
        </Label>
        <Input
          id={spawnedByFieldId}
          onChange={(event) => {
            const spawnedBy = event.target.value.trim();
            onChange({
              ...guard,
              spawnedBy: spawnedBy.length > 0 ? spawnedBy : undefined,
            });
          }}
          type="text"
          value={guard.spawnedBy ?? ""}
        />
        {resolveFieldError(fieldErrors, slotIndex, "spawnedBy") ? (
          <GuardFieldError
            message={
              resolveFieldError(fieldErrors, slotIndex, "spawnedBy") ?? ""
            }
          />
        ) : null}
      </div>
    </div>
  );
}

function GuardFieldError({ message }: { message: string }) {
  return <FormError>{message}</FormError>;
}

function resolveFieldError(
  fieldErrors: Record<string, string | undefined>,
  slotIndex: number,
  field: string,
): string | undefined {
  return (
    fieldErrors[`inputs[${slotIndex}].guard.${field}`] ??
    fieldErrors[`inputs[${slotIndex}].guards.${field}`]
  );
}

function resolveEditableInputGuard(
  guards: InputGuardBase[],
): InputGuard | undefined {
  const guard = guards[0];
  if (!guard) {
    return undefined;
  }

  if (!(INPUT_GUARD_TYPES as readonly string[]).includes(guard.type)) {
    return undefined;
  }

  return guard as InputGuard;
}
