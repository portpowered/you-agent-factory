import { useId } from "react";

import { Input, Select } from "../../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
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
import { CURRENT_SELECTION_FIELD_PANEL_CLASS } from "../../base/components/detail-card-shared";
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
      <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
        <h5 className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
          {messages.workstationInputGuardsHeading}
        </h5>
        <p className={cn("m-0 text-on-surface-variant", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.workstationInputGuardsEmpty}
        </p>
      </div>
    );
  }

  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <h5 className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
        {messages.workstationInputGuardsHeading}
      </h5>
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
    </div>
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
    <article
      aria-labelledby={`${rowId}-heading`}
      className="grid gap-2 rounded-lg border border-outline bg-surface-container-high p-3"
    >
      <div className="grid min-w-0 gap-1">
        <h6 className="m-0 text-sm text-on-surface" id={`${rowId}-heading`}>
          {messages.workstationInputSlotHeading(input.workType, input.state)}
        </h6>
        {guard ? (
          <p
            className={cn(
              "m-0 text-on-surface-subtle",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {messages.localizeInputGuardType(guard.type)} ·{" "}
            {formatInputGuardSummary(guard)}
          </p>
        ) : null}
      </div>
      <div className="grid gap-2">
        <label
          className={DASHBOARD_SUPPORTING_LABEL_CLASS}
          htmlFor={guardTypeFieldId}
        >
          {messages.workstationInputGuardTypeFieldLabel}
        </label>
        <Select
          className={DASHBOARD_BODY_TEXT_CLASS}
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
        </Select>
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
      <label
        className={DASHBOARD_SUPPORTING_LABEL_CLASS}
        htmlFor={matchInputFieldId}
      >
        {matchInputFieldLabel}
      </label>
      {peerWorkTypes.length === 0 ? (
        <p className={cn("m-0 text-on-surface-variant", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.workstationInputGuardPeersEmpty}
        </p>
      ) : (
        <Select
          className={DASHBOARD_BODY_TEXT_CLASS}
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
        </Select>
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
        <label
          className={DASHBOARD_SUPPORTING_LABEL_CLASS}
          htmlFor={parentInputFieldId}
        >
          {messages.inputGuardParentInputFieldLabel}
        </label>
        {peerWorkTypes.length === 0 ? (
          <p
            className={cn("m-0 text-on-surface-variant", DASHBOARD_BODY_TEXT_CLASS)}
          >
            {messages.workstationInputGuardPeersEmpty}
          </p>
        ) : (
          <Select
            className={DASHBOARD_BODY_TEXT_CLASS}
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
          </Select>
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
        <label
          className={DASHBOARD_SUPPORTING_LABEL_CLASS}
          htmlFor={spawnedByFieldId}
        >
          {messages.inputGuardSpawnedByFieldLabel}
        </label>
        <Input
          className={DASHBOARD_BODY_TEXT_CLASS}
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
  return (
    <p
      className={cn("m-0 text-on-error-container", DASHBOARD_SUPPORTING_TEXT_CLASS)}
      role="alert"
    >
      {message}
    </p>
  );
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
