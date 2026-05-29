export interface WorkerDetailMessages {
  argsFieldLabel: string;
  bodyFieldLabel: string;
  collapseAction: string;
  commandFieldLabel: string;
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  discardDraftAction: string;
  editableConfigurationCollapseActionLabel: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  editableConfigurationExpandActionLabel: string;
  editableConfigurationHeading: string;
  editableConfigurationArgsInvalid: string;
  editableConfigurationBodyRequired: string;
  editableConfigurationCommandRequired: string;
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationDirtyStatus: string;
  editableConfigurationDraftNote: string;
  editableConfigurationLoading: string;
  editableConfigurationModelProviderRequired: string;
  editableConfigurationModelRequired: string;
  editableConfigurationProviderRequired: string;
  editableConfigurationSaveAction: string;
  editableConfigurationSaveDisabledValidationDetail: string;
  editableConfigurationScriptCommandOrBodyRequired: string;
  editableConfigurationSharedImpactWarning: (
    workerName: string,
    workstationNames: string,
  ) => string;
  editableConfigurationSharedImpactWarningDetail: string;
  editableConfigurationValidationStatus: string;
  executorProviderLabel: string;
  expandAction: string;
  modelLabel: string;
  modelLocalityLabel: string;
  modelProviderLabel: string;
  notConfiguredOptionLabel: string;
  notConfiguredValue: string;
  providerFieldLabel: string;
  referencingWorkstationsEmpty: string;
  referencingWorkstationsHeading: string;
  summaryHeading: string;
  typeFieldLabel: string;
  typeLabel: string;
  unknownTypeValue: string;
  localizeExecutorProvider: (value: string) => string;
  localizeModelLocality: (value: string) => string;
  localizeModelProvider: (value: string) => string;
  localizeWorkerType: (value: string) => string;
}
