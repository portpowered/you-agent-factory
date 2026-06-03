import { useId } from "react";

import { MonacoGuardSelectorEditor } from "../../../../components/prompt-editor";
import {
  DashboardActionButton,
  Input,
  Select,
} from "../../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import {
  createDefaultWorkstationGuard,
  formatWorkstationGuardSummary,
  WORKSTATION_LEVEL_GUARD_TYPES,
  type WorkstationLevelGuard,
  type WorkstationLevelGuardType,
} from "../../../current-factory-definition/lib/workstation-guards";
import { CURRENT_SELECTION_FORM_FIELD_CLASS } from "../../base/components/detail-card-shared";
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
    <div className={CURRENT_SELECTION_FORM_FIELD_CLASS}>
      <h5 className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
        {messages.workstationGuardsHeading}
      </h5>
      {guards.length === 0 ? (
        <p
          className={cn(
            "m-0 text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
          {messages.workstationGuardsEmpty}
        </p>
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
        <label
          className={DASHBOARD_SUPPORTING_LABEL_CLASS}
          htmlFor={addGuardFieldId}
        >
          {messages.workstationGuardsAddLabel}
        </label>
        <Select
          className={DASHBOARD_BODY_TEXT_CLASS}
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
        </Select>
      </div>
    </div>
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
    <article
      aria-labelledby={`${rowId}-heading`}
      className="grid gap-2 rounded-lg border border-outline bg-surface-container-high p-3"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="grid min-w-0 gap-1">
          <h6 className="m-0 text-sm text-on-surface" id={`${rowId}-heading`}>
            {messages.localizeWorkstationGuardType(guard.type)}
          </h6>
          <p
            className={cn(
              "m-0 text-on-surface-subtle",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {formatWorkstationGuardSummary(guard)}
          </p>
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
        <label
          className={DASHBOARD_SUPPORTING_LABEL_CLASS}
          htmlFor={workstationFieldId}
        >
          {messages.visitCountGuardWorkstationFieldLabel}
        </label>
        {workstationOptionsState.status === "error" ? (
          <p
            className={cn(
              "m-0 text-on-error-container",
              DASHBOARD_BODY_TEXT_CLASS,
            )}
            role="alert"
          >
            {messages.editableConfigurationWorkstationUnavailablePrefix}{" "}
            {workstationOptionsState.message}
          </p>
        ) : null}
        {workstationOptionsState.status === "empty" ? (
          <p
            className={cn(
              "m-0 text-on-surface-variant",
              DASHBOARD_BODY_TEXT_CLASS,
            )}
          >
            {workstationOptionsState.message}
          </p>
        ) : null}
        {workstationOptionsState.status === "ready" ? (
          <Select
            className={DASHBOARD_BODY_TEXT_CLASS}
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
          </Select>
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
        <label
          className={DASHBOARD_SUPPORTING_LABEL_CLASS}
          htmlFor={maxVisitsFieldId}
        >
          {messages.visitCountGuardMaxVisitsFieldLabel}
        </label>
        <Input
          className={DASHBOARD_BODY_TEXT_CLASS}
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
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
        {messages.matchesFieldsGuardInputKeyFieldLabel}
      </span>
      <MonacoGuardSelectorEditor
        ariaDescribedBy={inputKeyError ? inputKeyErrorId : undefined}
        ariaInvalid={Boolean(inputKeyError)}
        ariaLabel={messages.matchesFieldsGuardInputKeyFieldLabel}
        className={DASHBOARD_BODY_TEXT_CLASS}
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
  return (
    <p
      className={cn(
        "m-0 text-on-error-container",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
      )}
      id={id}
      role="alert"
    >
      {message}
    </p>
  );
}

function resolveWorkstationGuardFieldError(
  fieldErrors: Record<string, string | undefined>,
  guardIndex: number,
  field: string,
): string | undefined {
  return fieldErrors[`guards[${guardIndex}].${field}`];
}
