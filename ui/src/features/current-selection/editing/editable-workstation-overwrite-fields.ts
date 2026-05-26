import { formatList } from "../../../components/ui/formatters";
import type { EditableWorkstationDraft } from "../../current-factory-definition/public";
import type { EditableWorkstationOverwriteField } from "../components/detail-card-types";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

export function resolveEditableWorkstationOverwriteFields(
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  const fields: EditableWorkstationOverwriteField[] = [];

  if (draft.behavior !== latestDefinitionDraft.behavior) {
    fields.push("behavior");
  }
  if (draft.prompt !== latestDefinitionDraft.prompt) {
    fields.push("prompt");
  }
  if (draft.workerName !== latestDefinitionDraft.workerName) {
    fields.push("worker");
  }
  if (draft.runnerName !== latestDefinitionDraft.runnerName) {
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
