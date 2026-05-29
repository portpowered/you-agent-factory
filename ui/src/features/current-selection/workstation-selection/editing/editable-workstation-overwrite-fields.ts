import { formatList } from "../../../../components/ui/formatters";
import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import type { EditableWorkstationOverwriteField } from "../lib/detail-card-types";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

export function resolveEditableWorkstationOverwriteFields(
  sessionStartDraft: EditableWorkstationDraft,
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  const fields: EditableWorkstationOverwriteField[] = [];

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
    sessionStartDraft.runnerName !== latestDefinitionDraft.runnerName &&
    draft.runnerName !== latestDefinitionDraft.runnerName
  ) {
    fields.push("runner");
  }

  return fields;
}

export function formatEditableOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkstationOverwriteField[],
  messages: Pick<
    WorkstationDetailMessages,
    "kindLabel" | "promptFieldLabel" | "runnerFieldLabel" | "workerFieldLabel"
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
    "kindLabel" | "promptFieldLabel" | "runnerFieldLabel" | "workerFieldLabel"
  >,
) {
  switch (field) {
    case "behavior":
      return messages.kindLabel.toLowerCase();
    case "prompt":
      return messages.promptFieldLabel.toLowerCase();
    case "runner":
      return messages.runnerFieldLabel.toLowerCase();
    case "worker":
      return messages.workerFieldLabel.toLowerCase();
  }
}
