import { useId } from "react";

import { MonacoGuardSelectorEditor } from "../../../../components/prompt-editor";
import {
  DashboardActionButton,
  DashboardLabel,
  DashboardText,
  FormDescription,
  FormError,
  Input,
  NativeSelect,
  SurfacePanel,
} from "../../../../components/ui";
import {
  createDefaultWorkstationGuard,
  formatWorkstationGuardSummary,
  WORKSTATION_LEVEL_GUARD_TYPES,
  type WorkstationLevelGuard,
  type WorkstationLevelGuardType,
} from "../../../current-factory-definition/lib/workstation-guards";
import { CurrentSelectionFormField } from "../../base/public";
import type { EditableWorkstationWorkstationOptionsState } from "../lib/detail-card-types";
import { useStableWorkstationGuardRowKeys } from "../lib/workstation-guard-row-keys";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

export function EditableConfigurationWorkstationGuardsField({
  fieldErrors = {},
  messages,
  onGuardsChange,
  guards,
  workstationOptionsState,
}: {
  fieldErrors?: Record<string, string | undefined>;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onGuardsChange: (guards: WorkstationLevelGuard[]) => void;
  guards: WorkstationLevelGuard[];
  workstationOptionsState: EditableWorkstationWorkstationOptionsState;
}) {
  const sectionId = useId();
  const addGuardFieldId = `${sectionId}-add-guard`;
  const guardRowKeys = useStableWorkstationGuardRowKeys(guards);

  return (
    <CurrentSelectionFormField>
      <DashboardLabel as="h5" className="m-0">
        {messages.workstationGuardsHeading}
      </DashboardLabel>
      {guards.length === 0 ? (
        <FormDescription variant="body">
          {messages.workstationGuardsEmpty}
        </FormDescription>
      ) : (
        <ul className="m-0 grid list-none gap-2 p-0">
          {guards.map((guard, index) => (
            <li key={guardRowKeys[index]}>
              <WorkstationGuardRow
                fieldErrors={fieldErrors}
                guard={guard}
                guardIndex={index}
                messages={messages}
                onChange={(nextGuard) => {
                  onGuardsChange(
                    guards.map((entry, entryIndex) =>
                      entryIndex === index ? nextGuard : entry,
                    ),
                  );
                }}
                onRemove={() => {
                  onGuardsChange(
                    guards.filter((_, entryIndex) => entryIndex !== index),
                  );
                }}
                workstationOptionsState={workstationOptionsState}
              />
            </li>
          ))}
        </ul>
      )}
      <div className="grid gap-2">
        <DashboardLabel as="label" htmlFor={addGuardFieldId}>
          {messages.workstationGuardsAddLabel}
        </DashboardLabel>
        <NativeSelect
          id={addGuardFieldId}
          onChange={(event) => {
            const nextType = event.target.value as WorkstationLevelGuardType;
            if (!nextType) {
              return;
            }
            onGuardsChange([
              ...guards,
              createDefaultWorkstationGuard(
                nextType,
                workstationOptionsState.status === "ready"
                  ? workstationOptionsState.options
                  : [],
              ),
            ]);
            event.target.value = "";
          }}
          value=""
        >
          <option value="">{messages.workstationGuardsAddPlaceholder}</option>
          {WORKSTATION_LEVEL_GUARD_TYPES.map((guardType) => (
            <option key={guardType} value={guardType}>
              {messages.localizeWorkstationGuardType(guardType)}
            </option>
          ))}
        </NativeSelect>
      </div>
    </CurrentSelectionFormField>
  );
}

function WorkstationGuardRow({
  fieldErrors,
  guard,
  guardIndex,
  messages,
  onChange,
  onRemove,
  workstationOptionsState,
}: {
  fieldErrors: Record<string, string | undefined>;
  guard: WorkstationLevelGuard;
  guardIndex: number;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (guard: WorkstationLevelGuard) => void;
  onRemove: () => void;
  workstationOptionsState: EditableWorkstationWorkstationOptionsState;
}) {
  const rowId = `editable-workstation-guard-${guardIndex}`;

  return (
    <SurfacePanel
      aria-labelledby={`${rowId}-heading`}
      asChild
      className="grid gap-2"
      radius="lg"
    >
      <article>
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="grid min-w-0 gap-1">
            <h6 className="m-0 text-sm text-on-surface" id={`${rowId}-heading`}>
              {messages.localizeWorkstationGuardType(guard.type)}
            </h6>
            <DashboardText
              className="m-0 text-on-surface-subtle"
              variant="supporting"
            >
              {formatWorkstationGuardSummary(guard)}
            </DashboardText>
          </div>
          <DashboardActionButton onClick={onRemove} type="button">
            {messages.workstationGuardsRemoveAction}
          </DashboardActionButton>
        </div>
        {guard.type === "VISIT_COUNT" ? (
          <VisitCountGuardFields
            fieldErrors={fieldErrors}
            guard={guard}
            guardIndex={guardIndex}
            messages={messages}
            onChange={onChange}
            rowId={rowId}
            workstationOptionsState={workstationOptionsState}
          />
        ) : null}
        {guard.type === "MATCHES_FIELDS" ? (
          <MatchesFieldsGuardFields
            fieldErrors={fieldErrors}
            guard={guard}
            guardIndex={guardIndex}
            messages={messages}
            onChange={onChange}
            rowId={rowId}
          />
        ) : null}
      </article>
    </SurfacePanel>
  );
}

function VisitCountGuardFields({
  fieldErrors,
  guard,
  guardIndex,
  messages,
  onChange,
  rowId,
  workstationOptionsState,
}: {
  fieldErrors: Record<string, string | undefined>;
  guard: Extract<WorkstationLevelGuard, { type: "VISIT_COUNT" }>;
  guardIndex: number;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (guard: WorkstationLevelGuard) => void;
  rowId: string;
  workstationOptionsState: EditableWorkstationWorkstationOptionsState;
}) {
  const workstationFieldId = `${rowId}-workstation`;
  const maxVisitsFieldId = `${rowId}-max-visits`;

  return (
    <div className="grid gap-2 md:grid-cols-2">
      <div className="grid gap-1">
        <DashboardLabel as="label" htmlFor={workstationFieldId}>
          {messages.visitCountGuardWorkstationFieldLabel}
        </DashboardLabel>
        {workstationOptionsState.status === "error" ? (
          <FormError>
            {messages.editableConfigurationWorkstationUnavailablePrefix}{" "}
            {workstationOptionsState.message}
          </FormError>
        ) : null}
        {workstationOptionsState.status === "empty" ? (
          <FormDescription variant="body">
            {workstationOptionsState.message}
          </FormDescription>
        ) : null}
        {workstationOptionsState.status === "ready" ? (
          <NativeSelect
            id={workstationFieldId}
            onChange={(event) => {
              onChange({
                ...guard,
                workstation: event.target.value,
              });
            }}
            value={guard.workstation ?? ""}
          >
            {workstationOptionsState.options.map((workstationName) => (
              <option key={workstationName} value={workstationName}>
                {workstationName}
              </option>
            ))}
          </NativeSelect>
        ) : null}
        {resolveWorkstationGuardFieldError(
          fieldErrors,
          guardIndex,
          "workstation",
        ) ? (
          <GuardFieldError
            message={
              resolveWorkstationGuardFieldError(
                fieldErrors,
                guardIndex,
                "workstation",
              ) ?? ""
            }
          />
        ) : null}
      </div>
      <div className="grid gap-1">
        <DashboardLabel as="label" htmlFor={maxVisitsFieldId}>
          {messages.visitCountGuardMaxVisitsFieldLabel}
        </DashboardLabel>
        <Input
          id={maxVisitsFieldId}
          inputMode="numeric"
          min={1}
          onChange={(event) => {
            const parsed = Number.parseInt(event.target.value, 10);
            onChange({
              ...guard,
              maxVisits: Number.isFinite(parsed) ? parsed : undefined,
            });
          }}
          type="number"
          value={guard.maxVisits ?? ""}
        />
        {resolveWorkstationGuardFieldError(
          fieldErrors,
          guardIndex,
          "maxVisits",
        ) ? (
          <GuardFieldError
            message={
              resolveWorkstationGuardFieldError(
                fieldErrors,
                guardIndex,
                "maxVisits",
              ) ?? ""
            }
          />
        ) : null}
      </div>
    </div>
  );
}

function MatchesFieldsGuardFields({
  fieldErrors,
  guard,
  guardIndex,
  messages,
  onChange,
  rowId,
}: {
  fieldErrors: Record<string, string | undefined>;
  guard: Extract<WorkstationLevelGuard, { type: "MATCHES_FIELDS" }>;
  guardIndex: number;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  onChange: (guard: WorkstationLevelGuard) => void;
  rowId: string;
}) {
  const inputKeyFieldId = `${rowId}-input-key`;
  const inputKeyErrorId = `${rowId}-input-key-error`;
  const inputKeyError = resolveWorkstationGuardFieldError(
    fieldErrors,
    guardIndex,
    "matchConfig.inputKey",
  );

  return (
    <div className="grid gap-1">
      <DashboardLabel>
        {messages.matchesFieldsGuardInputKeyFieldLabel}
      </DashboardLabel>
      <MonacoGuardSelectorEditor
        ariaDescribedBy={inputKeyError ? inputKeyErrorId : undefined}
        ariaInvalid={Boolean(inputKeyError)}
        ariaLabel={messages.matchesFieldsGuardInputKeyFieldLabel}
        hasError={Boolean(inputKeyError)}
        id={inputKeyFieldId}
        loadingMessage={
          messages.editableConfigurationGuardSelectorEditorLoading
        }
        modelPath={`inmemory://model/current-selection/workstation-guard-selector/${inputKeyFieldId}`}
        onChange={(nextInputKey) => {
          onChange({
            ...guard,
            matchConfig: { inputKey: nextInputKey },
          });
        }}
        startupErrorMessage={
          messages.editableConfigurationGuardSelectorEditorError
        }
        value={guard.matchConfig?.inputKey ?? ""}
      />
      {inputKeyError ? (
        <GuardFieldError id={inputKeyErrorId} message={inputKeyError} />
      ) : null}
    </div>
  );
}

function GuardFieldError({ id, message }: { id?: string; message: string }) {
  return <FormError id={id}>{message}</FormError>;
}

function resolveWorkstationGuardFieldError(
  fieldErrors: Record<string, string | undefined>,
  guardIndex: number,
  field: string,
): string | undefined {
  return fieldErrors[`guards[${guardIndex}].${field}`];
}
