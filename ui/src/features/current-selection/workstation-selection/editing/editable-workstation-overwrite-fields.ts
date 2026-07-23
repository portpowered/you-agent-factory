import { formatList } from "../../../../components/ui/formatters";
import { editableModelInvokeBindingsEqual } from "../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import type {
  EditableWorkstationCronDraft,
  EditableWorkstationDraft,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { editableWorkstationDraftNamesEqual } from "../../../current-factory-definition/lib/workstation-guards";
import type { EditableWorkstationOverwriteField } from "../lib/keys/detail-card-types";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

export function resolveEditableWorkstationOverwriteFields(
  sessionStartDraft: EditableWorkstationDraft,
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  const fields: EditableWorkstationOverwriteField[] = [];

  if (
    !editableWorkstationDraftNamesEqual(
      sessionStartDraft,
      latestDefinitionDraft,
    ) &&
    !editableWorkstationDraftNamesEqual(draft, latestDefinitionDraft)
  ) {
    fields.push("name");
  }
  if (
    sessionStartDraft.behavior !== latestDefinitionDraft.behavior &&
    draft.behavior !== latestDefinitionDraft.behavior
  ) {
    fields.push("behavior");
  }
  if (
    sessionStartDraft.prompt !== latestDefinitionDraft.prompt &&
    draft.prompt !== latestDefinitionDraft.prompt
  ) {
    fields.push("prompt");
  }
  if (
    sessionStartDraft.workerName !== latestDefinitionDraft.workerName &&
    draft.workerName !== latestDefinitionDraft.workerName
  ) {
    fields.push("worker");
  }
  if (
    sessionStartDraft.workstationType !==
      latestDefinitionDraft.workstationType &&
    draft.workstationType !== latestDefinitionDraft.workstationType
  ) {
    fields.push("workstationType");
  }
  if (
    sessionStartDraft.operation !== latestDefinitionDraft.operation &&
    draft.operation !== latestDefinitionDraft.operation
  ) {
    fields.push("operation");
  }
  if (
    !editableModelInvokeBindingsEqual(
      sessionStartDraft.operationBindings,
      latestDefinitionDraft.operationBindings,
    ) &&
    !editableModelInvokeBindingsEqual(
      draft.operationBindings,
      latestDefinitionDraft.operationBindings,
    )
  ) {
    fields.push("operationBindings");
  }
  if (
    sessionStartDraft.runnerName !== latestDefinitionDraft.runnerName &&
    draft.runnerName !== latestDefinitionDraft.runnerName
  ) {
    fields.push("runner");
  }

  fields.push(
    ...resolveEditableWorkstationCronOverwriteFields(
      sessionStartDraft,
      draft,
      latestDefinitionDraft,
    ),
  );

  return fields;
}

function resolveEditableWorkstationCronOverwriteFields(
  sessionStartDraft: EditableWorkstationDraft,
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  if (
    sessionStartDraft.behavior !== "CRON" ||
    draft.behavior !== "CRON" ||
    latestDefinitionDraft.behavior !== "CRON"
  ) {
    return [];
  }

  const sessionStartCron = sessionStartDraft.cron;
  const draftCron = draft.cron;
  const latestCron = latestDefinitionDraft.cron;
  if (!sessionStartCron || !draftCron || !latestCron) {
    return [];
  }

  const fields: EditableWorkstationOverwriteField[] = [];
  appendCronOverwriteField(
    fields,
    "cronSchedule",
    sessionStartCron,
    draftCron,
    latestCron,
    "schedule",
  );
  appendCronOverwriteField(
    fields,
    "cronTriggerAtStart",
    sessionStartCron,
    draftCron,
    latestCron,
    "triggerAtStart",
  );
  appendCronOverwriteField(
    fields,
    "cronJitter",
    sessionStartCron,
    draftCron,
    latestCron,
    "jitter",
  );
  appendCronOverwriteField(
    fields,
    "cronExpiryWindow",
    sessionStartCron,
    draftCron,
    latestCron,
    "expiryWindow",
  );
  return fields;
}

function appendCronOverwriteField(
  fields: EditableWorkstationOverwriteField[],
  field: EditableWorkstationOverwriteField,
  sessionStartCron: EditableWorkstationCronDraft,
  draftCron: EditableWorkstationCronDraft,
  latestCron: EditableWorkstationCronDraft,
  key: keyof EditableWorkstationCronDraft,
) {
  if (
    sessionStartCron[key] !== latestCron[key] &&
    draftCron[key] !== latestCron[key]
  ) {
    fields.push(field);
  }
}

export function formatEditableOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkstationOverwriteField[],
  messages: Pick<
    WorkstationDetailMessages,
    | "cronExpiryWindowFieldLabel"
    | "cronJitterFieldLabel"
    | "cronScheduleFieldLabel"
    | "cronTriggerAtStartFieldLabel"
    | "kindLabel"
    | "modelInvokeBindingsFieldLabel"
    | "modelInvokeOperationFieldLabel"
    | "promptFieldLabel"
    | "runnerFieldLabel"
    | "workerFieldLabel"
    | "workstationNameFieldLabel"
    | "workstationTypeLabel"
  >,
) {
  return formatList(
    overwriteFieldNames.map((field) => fieldLabel(field, messages)),
  );
}

function fieldLabel(
  field: EditableWorkstationOverwriteField,
  messages: Pick<
    WorkstationDetailMessages,
    | "cronExpiryWindowFieldLabel"
    | "cronJitterFieldLabel"
    | "cronScheduleFieldLabel"
    | "cronTriggerAtStartFieldLabel"
    | "kindLabel"
    | "modelInvokeBindingsFieldLabel"
    | "modelInvokeOperationFieldLabel"
    | "promptFieldLabel"
    | "runnerFieldLabel"
    | "workerFieldLabel"
    | "workstationNameFieldLabel"
    | "workstationTypeLabel"
  >,
) {
  switch (field) {
    case "name":
      return messages.workstationNameFieldLabel.toLowerCase();
    case "workstationType":
      return messages.workstationTypeLabel.toLowerCase();
    case "behavior":
      return messages.kindLabel.toLowerCase();
    case "prompt":
      return messages.promptFieldLabel.toLowerCase();
    case "runner":
      return messages.runnerFieldLabel.toLowerCase();
    case "worker":
      return messages.workerFieldLabel.toLowerCase();
    case "operation":
      return messages.modelInvokeOperationFieldLabel.toLowerCase();
    case "operationBindings":
      return messages.modelInvokeBindingsFieldLabel.toLowerCase();
    case "cronSchedule":
      return messages.cronScheduleFieldLabel.toLowerCase();
    case "cronTriggerAtStart":
      return messages.cronTriggerAtStartFieldLabel.toLowerCase();
    case "cronJitter":
      return messages.cronJitterFieldLabel.toLowerCase();
    case "cronExpiryWindow":
      return messages.cronExpiryWindowFieldLabel.toLowerCase();
  }
}
