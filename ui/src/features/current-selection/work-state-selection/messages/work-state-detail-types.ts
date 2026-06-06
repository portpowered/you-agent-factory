import type { EditableWorkStateDraft } from "../../../current-factory-definition/lib/work-state-editable-values";

export interface WorkStateDetailMessages {
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  discardDraftAction: string;
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationDirtyStatus: string;
  editableConfigurationDraftNote: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  collapseAction: string;
  editableConfigurationCollapseActionLabel: string;
  editableConfigurationExpandActionLabel: string;
  editableConfigurationHeading: string;
  editableConfigurationLoading: string;
  expandAction: string;
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
  summaryHeading: string;
  typeFieldLabel: string;
}
