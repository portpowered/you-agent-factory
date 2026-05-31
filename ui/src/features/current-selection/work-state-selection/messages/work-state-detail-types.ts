export interface WorkStateDetailMessages {
  topologyDeleteAction: (workTypeName: string, stateName: string) => string;
  topologyDeleteBlockedPrefix: string;
  topologyDeleteHeading: string;
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  editableConfigurationContractInvalidPrefix: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  editableConfigurationNameDuplicate: (stateName: string) => string;
  editableConfigurationNameRequired: string;
}
