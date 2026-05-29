import { formatList } from "../../../../components/ui/formatters";
import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import type { EditableWorkerOverwriteField } from "../lib/detail-card-types";
import type { WorkerDetailMessages } from "../messages/worker-detail-types";

export function resolveEditableWorkerOverwriteFields(
  sessionStartDraft: EditableWorkerDraft,
  draft: EditableWorkerDraft,
  latestDefinitionDraft: EditableWorkerDraft,
): EditableWorkerOverwriteField[] {
  const fields: EditableWorkerOverwriteField[] = [];

  if (
    sessionStartDraft.type !== latestDefinitionDraft.type &&
    draft.type !== latestDefinitionDraft.type
  ) {
    fields.push("type");
  }
  if (
    sessionStartDraft.modelProvider !== latestDefinitionDraft.modelProvider &&
    draft.modelProvider !== latestDefinitionDraft.modelProvider
  ) {
    fields.push("modelProvider");
  }
  if (
    sessionStartDraft.model !== latestDefinitionDraft.model &&
    draft.model !== latestDefinitionDraft.model
  ) {
    fields.push("model");
  }
  if (
    sessionStartDraft.modelLocality !== latestDefinitionDraft.modelLocality &&
    draft.modelLocality !== latestDefinitionDraft.modelLocality
  ) {
    fields.push("modelLocality");
  }
  if (
    sessionStartDraft.executorProvider !== latestDefinitionDraft.executorProvider &&
    draft.executorProvider !== latestDefinitionDraft.executorProvider
  ) {
    fields.push("executorProvider");
  }
  if (
    sessionStartDraft.command !== latestDefinitionDraft.command &&
    draft.command !== latestDefinitionDraft.command
  ) {
    fields.push("command");
  }
  if (
    sessionStartDraft.argsText !== latestDefinitionDraft.argsText &&
    draft.argsText !== latestDefinitionDraft.argsText
  ) {
    fields.push("args");
  }
  if (
    sessionStartDraft.body !== latestDefinitionDraft.body &&
    draft.body !== latestDefinitionDraft.body
  ) {
    fields.push("body");
  }
  if (
    sessionStartDraft.provider !== latestDefinitionDraft.provider &&
    draft.provider !== latestDefinitionDraft.provider
  ) {
    fields.push("provider");
  }

  return fields;
}

export function formatEditableWorkerOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkerOverwriteField[],
  messages: Pick<
    WorkerDetailMessages,
    | "argsFieldLabel"
    | "bodyFieldLabel"
    | "commandFieldLabel"
    | "executorProviderLabel"
    | "modelLabel"
    | "modelLocalityLabel"
    | "modelProviderLabel"
    | "providerFieldLabel"
    | "typeFieldLabel"
  >,
) {
  return formatList(
    overwriteFieldNames.map((field) => fieldLabel(field, messages)),
  );
}

function fieldLabel(
  field: EditableWorkerOverwriteField,
  messages: Pick<
    WorkerDetailMessages,
    | "argsFieldLabel"
    | "bodyFieldLabel"
    | "commandFieldLabel"
    | "executorProviderLabel"
    | "modelLabel"
    | "modelLocalityLabel"
    | "modelProviderLabel"
    | "providerFieldLabel"
    | "typeFieldLabel"
  >,
) {
  switch (field) {
    case "type":
      return messages.typeFieldLabel.toLowerCase();
    case "modelProvider":
      return messages.modelProviderLabel.toLowerCase();
    case "model":
      return messages.modelLabel.toLowerCase();
    case "modelLocality":
      return messages.modelLocalityLabel.toLowerCase();
    case "executorProvider":
      return messages.executorProviderLabel.toLowerCase();
    case "command":
      return messages.commandFieldLabel.toLowerCase();
    case "args":
      return messages.argsFieldLabel.toLowerCase();
    case "body":
      return messages.bodyFieldLabel.toLowerCase();
    case "provider":
      return messages.providerFieldLabel.toLowerCase();
  }
}
