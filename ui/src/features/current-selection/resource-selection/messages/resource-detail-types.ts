export interface ResourceDetailMessages {
  backendFieldLabel: string;
  capacityFieldLabel: string;
  collapseAction: string;
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  editableConfigurationCapacityInvalid: string;
  editableConfigurationCollapseActionLabel: string;
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationDirtyStatus: string;
  editableConfigurationDraftNote: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  editableConfigurationExpandActionLabel: string;
  editableConfigurationHeading: string;
  editableConfigurationLoading: string;
  editableConfigurationNameDuplicate: (resourceName: string) => string;
  editableConfigurationNameRequired: string;
  editableConfigurationOverwriteWarning: (fields: string) => string;
  editableConfigurationOverwriteWarningDetail: string;
  editableConfigurationSaveDisabledValidationDetail: string;
  editableConfigurationServerFieldChangedHint: string;
  editableConfigurationSharedImpactWarning: (
    resourceName: string,
    workerNames: string,
    workstationNames: string,
  ) => string;
  editableConfigurationSharedImpactWarningDetail: string;
  editableConfigurationValidationStatus: string;
  expandAction: string;
  loadPolicyFieldLabel: string;
  modelFieldLabel: string;
  nameFieldLabel: string;
  notConfiguredValue: string;
  providerFieldLabel: string;
  referencingWorkersEmpty: string;
  referencingWorkersHeading: string;
  referencingWorkstationsEmpty: string;
  referencingWorkstationsHeading: string;
  resetToLatestAction: string;
  summaryHeading: string;
  tokenCountFieldLabel: string;
  typeFieldLabel: string;
  unknownTypeValue: string;
  localizeResourceType: (value: string) => string;
}
