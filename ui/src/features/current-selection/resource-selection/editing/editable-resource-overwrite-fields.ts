import { formatList } from "../../../../components/ui/formatters";
import type { EditableResourceDraft } from "../../../current-factory-definition/lib/resource-editable-values";
import type { EditableResourceOverwriteField } from "../lib/detail-card-types";
import type { ResourceDetailMessages } from "../messages/resource-detail-types";

export function resolveEditableResourceOverwriteFields(
  sessionStartDraft: EditableResourceDraft,
  draft: EditableResourceDraft,
  latestDefinitionDraft: EditableResourceDraft,
): EditableResourceOverwriteField[] {
  const fields: EditableResourceOverwriteField[] = [];

  if (
    sessionStartDraft.type !== latestDefinitionDraft.type &&
    draft.type !== latestDefinitionDraft.type
  ) {
    fields.push("type");
  }
  if (
    sessionStartDraft.capacityText !== latestDefinitionDraft.capacityText &&
    draft.capacityText !== latestDefinitionDraft.capacityText
  ) {
    fields.push("capacity");
  }
  if (
    sessionStartDraft.model !== latestDefinitionDraft.model &&
    draft.model !== latestDefinitionDraft.model
  ) {
    fields.push("model");
  }
  if (
    sessionStartDraft.backend !== latestDefinitionDraft.backend &&
    draft.backend !== latestDefinitionDraft.backend
  ) {
    fields.push("backend");
  }
  if (
    sessionStartDraft.loadPolicy !== latestDefinitionDraft.loadPolicy &&
    draft.loadPolicy !== latestDefinitionDraft.loadPolicy
  ) {
    fields.push("loadPolicy");
  }
  if (
    sessionStartDraft.provider !== latestDefinitionDraft.provider &&
    draft.provider !== latestDefinitionDraft.provider
  ) {
    fields.push("provider");
  }
  if (
    sessionStartDraft.name !== latestDefinitionDraft.name &&
    draft.name !== latestDefinitionDraft.name
  ) {
    fields.push("name");
  }

  return fields;
}

export function formatEditableResourceOverwriteFieldLabels(
  overwriteFieldNames: EditableResourceOverwriteField[],
  messages: Pick<
    ResourceDetailMessages,
    | "backendFieldLabel"
    | "capacityFieldLabel"
    | "loadPolicyFieldLabel"
    | "modelFieldLabel"
    | "nameFieldLabel"
    | "providerFieldLabel"
    | "typeFieldLabel"
  >,
) {
  return formatList(
    overwriteFieldNames.map((field) => fieldLabel(field, messages)),
  );
}

function fieldLabel(
  field: EditableResourceOverwriteField,
  messages: Pick<
    ResourceDetailMessages,
    | "backendFieldLabel"
    | "capacityFieldLabel"
    | "loadPolicyFieldLabel"
    | "modelFieldLabel"
    | "nameFieldLabel"
    | "providerFieldLabel"
    | "typeFieldLabel"
  >,
) {
  switch (field) {
    case "type":
      return messages.typeFieldLabel.toLowerCase();
    case "capacity":
      return messages.capacityFieldLabel.toLowerCase();
    case "model":
      return messages.modelFieldLabel.toLowerCase();
    case "backend":
      return messages.backendFieldLabel.toLowerCase();
    case "loadPolicy":
      return messages.loadPolicyFieldLabel.toLowerCase();
    case "provider":
      return messages.providerFieldLabel.toLowerCase();
    case "name":
      return messages.nameFieldLabel.toLowerCase();
  }
}
