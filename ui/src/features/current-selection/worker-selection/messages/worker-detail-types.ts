export interface WorkerDetailMessages {
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  executorProviderLabel: string;
  modelLabel: string;
  modelProviderLabel: string;
  notConfiguredValue: string;
  referencingWorkstationsEmpty: string;
  referencingWorkstationsHeading: string;
  summaryHeading: string;
  typeLabel: string;
  unknownTypeValue: string;
  localizeExecutorProvider: (value: string) => string;
  localizeModelProvider: (value: string) => string;
  localizeWorkerType: (value: string) => string;
}
