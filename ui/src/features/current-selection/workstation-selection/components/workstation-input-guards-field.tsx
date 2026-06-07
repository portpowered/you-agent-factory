import { useId } from "react";

import {
  DashboardLabel,
  DashboardText,
  FormDescription,
  FormError,
  Input,
  NativeSelect,
  SurfacePanel,
} from "../../../../components/ui";
import type { EditableWorkstationInputDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  createDefaultInputGuard,
  formatInputGuardSummary,
  INPUT_GUARD_TYPES,
  type InputGuard,
  type InputGuardBase,
  type InputGuardType,
  resolvePeerInputWorkTypes,
  setEditableInputSlotGuard,
} from "../../../current-factory-definition/lib/workstation-guards";
import { CurrentSelectionFormField } from "../../base/public";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

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
        <DashboardLabel as="h5" className="m-0">
          {messages.workstationInputGuardsHeading}
        </DashboardLabel>
        <FormDescription variant="body">
          {messages.workstationInputGuardsEmpty}
        </FormDescription>
      </CurrentSelectionFormField>
    );
  }

  return (
    <CurrentSelectionFormField>
      <DashboardLabel as="h5" className="m-0">
        {messages.workstationInputGuardsHeading}
      </DashboardLabel>
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
            <DashboardText
              className="m-0 text-on-surface-subtle"
              variant="supporting"
            >
              {messages.localizeInputGuardType(guard.type)} ·{" "}
              {formatInputGuardSummary(guard)}
            </DashboardText>
          ) : null}
        </div>
        <div className="grid gap-2">
          <DashboardLabel as="label" htmlFor={guardTypeFieldId}>
            {messages.workstationInputGuardTypeFieldLabel}
          </DashboardLabel>
          <NativeSelect
            id={guardTypeFieldId}
            onChange={(event) => {
              const nextType = event.target.value as InputGuardType | "";
              if (!nextType) {
                onChange(setEditableInputSlotGuard(input, null));
                return;
              }

              onChange(
                setEditableInputSlotGuard(
                  input,
                  createDefaultInputGuard(nextType, peerWorkTypes),
                ),
              );
            }}
            value={guardTypeValue}
          >
            <option value="">{messages.workstationInputGuardNoneOption}</option>
            {INPUT_GUARD_TYPES.map((guardType) => (
              <option key={guardType} value={guardType}>
                {messages.localizeInputGuardType(guardType)}
              </option>
            ))}
          </NativeSelect>
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
      <DashboardLabel as="label" htmlFor={matchInputFieldId}>
        {matchInputFieldLabel}
      </DashboardLabel>
      {peerWorkTypes.length === 0 ? (
        <FormDescription variant="body">
          {messages.workstationInputGuardPeersEmpty}
        </FormDescription>
      ) : (
        <NativeSelect
          id={matchInputFieldId}
          onChange={(event) => {
            onChange({
              ...guard,
              matchInput: event.target.value,
            });
          }}
          value={guard.matchInput ?? ""}
        >
          {peerWorkTypes.map((workType) => (
            <option key={workType} value={workType}>
              {workType}
            </option>
          ))}
        </NativeSelect>
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
        <DashboardLabel as="label" htmlFor={parentInputFieldId}>
          {messages.inputGuardParentInputFieldLabel}
        </DashboardLabel>
        {peerWorkTypes.length === 0 ? (
          <FormDescription variant="body">
            {messages.workstationInputGuardPeersEmpty}
          </FormDescription>
        ) : (
          <NativeSelect
            id={parentInputFieldId}
            onChange={(event) => {
              onChange({
                ...guard,
                parentInput: event.target.value,
              });
            }}
            value={guard.parentInput ?? ""}
          >
            {peerWorkTypes.map((workType) => (
              <option key={workType} value={workType}>
                {workType}
              </option>
            ))}
          </NativeSelect>
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
        <DashboardLabel as="label" htmlFor={spawnedByFieldId}>
          {messages.inputGuardSpawnedByFieldLabel}
        </DashboardLabel>
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
