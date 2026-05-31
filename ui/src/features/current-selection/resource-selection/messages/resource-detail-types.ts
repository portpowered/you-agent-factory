export interface ResourceDetailMessages {
  backendFieldLabel: string;
  capacityFieldLabel: string;
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  loadPolicyFieldLabel: string;
  modelFieldLabel: string;
  nameFieldLabel: string;
  notConfiguredValue: string;
  providerFieldLabel: string;
  referencingWorkersEmpty: string;
  referencingWorkersHeading: string;
  referencingWorkstationsEmpty: string;
  referencingWorkstationsHeading: string;
  summaryHeading: string;
  tokenCountFieldLabel: string;
  typeFieldLabel: string;
  unknownTypeValue: string;
  localizeResourceType: (value: string) => string;
}
