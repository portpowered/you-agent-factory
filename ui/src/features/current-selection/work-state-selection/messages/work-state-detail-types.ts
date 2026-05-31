import type { EditableWorkStateDraft } from "../../../current-factory-definition/lib/work-state-editable-values";

export interface WorkStateDetailMessages {
  topologyDeleteAction: (workTypeName: string, stateName: string) => string;
  topologyDeleteBlockedPrefix: string;
  topologyDeleteHeading: string;
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  discardDraftAction: string;
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationDirtyStatus: string;
  editableConfigurationDraftNote: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  editableConfigurationHeading: string;
  editableConfigurationLoading: string;
  editableConfigurationNameDuplicate: (stateName: string) => string;
  editableConfigurationNameRequired: string;
  editableConfigurationSaveAction: string;
  editableConfigurationSaveBusyAction: string;
  editableConfigurationSaveDisabledValidationDetail: string;
  editableConfigurationSaveErrorPrefix: string;
  editableConfigurationSaveFallbackError: string;
  editableConfigurationSaveStaleVersionDetail: string;
  editableConfigurationSaveSuccess: (stateName: string) => string;
  editableConfigurationValidationStatus: string;
  localizeWorkStateType: (stateType: EditableWorkStateDraft["type"]) => string;
  nameFieldLabel: string;
  typeFieldLabel: string;
}
