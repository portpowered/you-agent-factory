import { formatList } from "../../../../components/ui/formatters";
import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import type { EditableWorkerOverwriteField } from "../lib/detail-card-types";
import type { WorkerDetailMessages } from "../messages/worker-detail-types";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: overwrite detection compares every editable worker field in one pass.
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
    sessionStartDraft.executorProvider !==
      latestDefinitionDraft.executorProvider &&
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
  if (
    sessionStartDraft.authSecretRef !== latestDefinitionDraft.authSecretRef &&
    draft.authSecretRef !== latestDefinitionDraft.authSecretRef
  ) {
    fields.push("authSecretRef");
  }
  if (
    sessionStartDraft.linearPollInterval !==
      latestDefinitionDraft.linearPollInterval &&
    draft.linearPollInterval !== latestDefinitionDraft.linearPollInterval
  ) {
    fields.push("linearPollInterval");
  }
  if (
    sessionStartDraft.linearTeamIdsText !==
      latestDefinitionDraft.linearTeamIdsText &&
    draft.linearTeamIdsText !== latestDefinitionDraft.linearTeamIdsText
  ) {
    fields.push("linearTeamIds");
  }
  if (
    sessionStartDraft.linearStateIdsText !==
      latestDefinitionDraft.linearStateIdsText &&
    draft.linearStateIdsText !== latestDefinitionDraft.linearStateIdsText
  ) {
    fields.push("linearStateIds");
  }
  if (
    sessionStartDraft.linearMappingWorkType !==
      latestDefinitionDraft.linearMappingWorkType &&
    draft.linearMappingWorkType !== latestDefinitionDraft.linearMappingWorkType
  ) {
    fields.push("linearMappingWorkType");
  }
  if (
    sessionStartDraft.linearMappingState !==
      latestDefinitionDraft.linearMappingState &&
    draft.linearMappingState !== latestDefinitionDraft.linearMappingState
  ) {
    fields.push("linearMappingState");
  }
  if (
    sessionStartDraft.linearClaimAssigneeField !==
      latestDefinitionDraft.linearClaimAssigneeField &&
    draft.linearClaimAssigneeField !==
      latestDefinitionDraft.linearClaimAssigneeField
  ) {
    fields.push("linearClaimAssigneeField");
  }
  if (
    sessionStartDraft.skipPermissions !==
      latestDefinitionDraft.skipPermissions &&
    draft.skipPermissions !== latestDefinitionDraft.skipPermissions
  ) {
    fields.push("skipPermissions");
  }
  if (
    sessionStartDraft.stopToken !== latestDefinitionDraft.stopToken &&
    draft.stopToken !== latestDefinitionDraft.stopToken
  ) {
    fields.push("stopToken");
  }
  if (
    (sessionStartDraft.timeoutAmount !== latestDefinitionDraft.timeoutAmount ||
      sessionStartDraft.timeoutUnit !== latestDefinitionDraft.timeoutUnit) &&
    (draft.timeoutAmount !== latestDefinitionDraft.timeoutAmount ||
      draft.timeoutUnit !== latestDefinitionDraft.timeoutUnit)
  ) {
    fields.push("timeout");
  }
  if (
    sessionStartDraft.name !== latestDefinitionDraft.name &&
    draft.name !== latestDefinitionDraft.name
  ) {
    fields.push("name");
  }

  return fields;
}

export function formatEditableWorkerOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkerOverwriteField[],
  messages: Pick<
    WorkerDetailMessages,
    | "argsFieldLabel"
    | "authSecretRefFieldLabel"
    | "bodyFieldLabel"
    | "commandFieldLabel"
    | "executorProviderLabel"
    | "linearClaimAssigneeFieldLabel"
    | "linearMappingStateFieldLabel"
    | "linearMappingWorkTypeFieldLabel"
    | "linearPollIntervalFieldLabel"
    | "linearStateIdsFieldLabel"
    | "linearTeamIdsFieldLabel"
    | "modelLabel"
    | "modelLocalityLabel"
    | "modelProviderLabel"
    | "nameFieldLabel"
    | "providerFieldLabel"
    | "skipPermissionsFieldLabel"
    | "stopTokenFieldLabel"
    | "timeoutFieldLabel"
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
    | "authSecretRefFieldLabel"
    | "bodyFieldLabel"
    | "commandFieldLabel"
    | "executorProviderLabel"
    | "linearClaimAssigneeFieldLabel"
    | "linearMappingStateFieldLabel"
    | "linearMappingWorkTypeFieldLabel"
    | "linearPollIntervalFieldLabel"
    | "linearStateIdsFieldLabel"
    | "linearTeamIdsFieldLabel"
    | "modelLabel"
    | "modelLocalityLabel"
    | "modelProviderLabel"
    | "nameFieldLabel"
    | "providerFieldLabel"
    | "skipPermissionsFieldLabel"
    | "stopTokenFieldLabel"
    | "timeoutFieldLabel"
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
    case "authSecretRef":
      return messages.authSecretRefFieldLabel.toLowerCase();
    case "linearPollInterval":
      return messages.linearPollIntervalFieldLabel.toLowerCase();
    case "linearTeamIds":
      return messages.linearTeamIdsFieldLabel.toLowerCase();
    case "linearStateIds":
      return messages.linearStateIdsFieldLabel.toLowerCase();
    case "linearMappingWorkType":
      return messages.linearMappingWorkTypeFieldLabel.toLowerCase();
    case "linearMappingState":
      return messages.linearMappingStateFieldLabel.toLowerCase();
    case "linearClaimAssigneeField":
      return messages.linearClaimAssigneeFieldLabel.toLowerCase();
    case "skipPermissions":
      return messages.skipPermissionsFieldLabel.toLowerCase();
    case "stopToken":
      return messages.stopTokenFieldLabel.toLowerCase();
    case "timeout":
      return messages.timeoutFieldLabel.toLowerCase();
    case "name":
      return messages.nameFieldLabel.toLowerCase();
  }
}
