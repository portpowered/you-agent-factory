export interface WorkstationDetailMessages {
  activeRunsLabel: string;
  activeWorkEmpty: string;
  activeWorkHeading: string;
  collapseAction: string;
  editableConfigurationEmpty: string;
  editableConfigurationErrorPrefix: string;
  editableConfigurationCollapseActionLabel: string;
  editableConfigurationExpandActionLabel: string;
  editableConfigurationHeading: string;
  editableConfigurationLoading: string;
  editableConfigurationModelSharedWorkerHint: string;
  editableConfigurationNameDuplicate: (workstationName: string) => string;
  editableConfigurationNameRequired: string;
  editableConfigurationResetAction: string;
  editableConfigurationServerFieldChangedHint: string;
  editableConfigurationOverwriteWarning: (fields: string) => string;
  editableConfigurationOverwriteWarningDetail: string;
  editableConfigurationSaveAction: string;
  editableConfigurationSaveBusyAction: string;
  editableConfigurationSaveConfirmationCancelAction: string;
  editableConfigurationSaveConfirmationConfirmAction: string;
  editableConfigurationSaveConfirmationDescription: string;
  editableConfigurationSaveConflictConfirmationDescription: (
    fields: string,
  ) => string;
  editableConfigurationSaveConfirmationTitle: string;
  editableConfigurationSaveErrorPrefix: string;
  editableConfigurationSaveStaleVersionDetail: string;
  editableConfigurationSaveSuccess: (workstationName: string) => string;
  editableConfigurationValidationStatus: string;
  editableConfigurationBehaviorPollerHint: string;
  editableConfigurationBehaviorPollerWorkerUnsupported: string;
  editableConfigurationPromptRequired: string;
  editableConfigurationPromptEditorLoading: string;
  editableConfigurationPromptEditorError: string;
  editableConfigurationPromptValidationLoading: string;
  editableConfigurationPromptValidationFallbackError: string;
  editableConfigurationPromptValidationErrorPrefix: string;
  editableConfigurationPromptDiagnosticsSummary: string;
  editableConfigurationPromptFieldHint: string;
  editableConfigurationPromptDiagnosticsHeading: string;
  editableConfigurationPromptSyntaxDiagnosticLabel: string;
  editableConfigurationPromptVariableDiagnosticLabel: string;
  editableConfigurationPromptHelpLoading: string;
  editableConfigurationPromptHelpEmpty: string;
  editableConfigurationPromptHelpFallbackError: string;
  editableConfigurationPromptHelpErrorPrefix: string;
  editableConfigurationPromptAutocompleteSummary: (
    variableCount: number,
    inputCount: number,
  ) => string;
  editableConfigurationPromptAutocompleteDetail: string;
  editableConfigurationPromptHelpCollapseActionLabel: string;
  editableConfigurationPromptHelpExpandActionLabel: string;
  editableConfigurationPromptAvailableVariablesHeading: string;
  editableConfigurationPromptUnavailableAccessHeading: string;
  editableConfigurationPromptResizeHandleLabel: string;
  editableConfigurationSaveFallbackError: string;
  editableConfigurationWorkerMissing: string;
  editableConfigurationWorkerOptionsEmpty: string;
  editableConfigurationWorkerRequired: string;
  editableConfigurationSharedWorkerScopeHint: (
    workerName: string,
    workstationNames: string,
  ) => string;
  editableConfigurationWorkerUnavailable: string;
  editableConfigurationWorkerUnavailablePrefix: string;
  editableConfigurationModelInvokeBindingDuplicate: (
    slotName: string,
  ) => string;
  editableConfigurationModelInvokeBindingRequired: (slotName: string) => string;
  editableConfigurationModelInvokeBindingsSummary: string;
  editableConfigurationModelInvokeOperationInvalid: string;
  editableConfigurationModelInvokeOperationMissing: string;
  editableConfigurationModelInvokeOperationOptionsEmpty: string;
  editableConfigurationModelInvokeOperationRequired: string;
  editableConfigurationModelInvokeWorkerOptionsEmpty: string;
  editableConfigurationModelInvokeWorkerRequired: string;
  modelInvokeBindingConfigContentFieldLabel: string;
  modelInvokeBindingDefaultContentFieldLabel: string;
  modelInvokeBindingsEmpty: string;
  modelInvokeBindingsFieldHint: string;
  modelInvokeBindingsFieldLabel: string;
  modelInvokeBindingOptionalSlotLabel: string;
  modelInvokeBindingRequiredSlotLabel: string;
  modelInvokeBindingSelectorLabelFieldLabel: string;
  modelInvokeBindingSelectorRoleFieldLabel: string;
  modelInvokeBindingSelectorSlotFieldLabel: string;
  modelInvokeBindingSelectorTypeFieldLabel: string;
  modelInvokeBindingSelectorTypeNoneOption: string;
  modelInvokeBindingSlotHeading: (
    slotName: string,
    requirement: string,
  ) => string;
  modelInvokeOperationFieldLabel: string;
  editableConfigurationCronExpiryWindowInvalid: (value: string) => string;
  editableConfigurationCronJitterInvalid: (value: string) => string;
  editableConfigurationCronScheduleInvalid: (
    schedule: string,
    detail: string,
  ) => string;
  editableConfigurationCronScheduleRequired: string;
  cronExpiryWindowFieldHint: string;
  cronExpiryWindowFieldLabel: string;
  cronJitterFieldHint: string;
  cronJitterFieldLabel: string;
  cronScheduleFieldHint: string;
  cronScheduleFieldLabel: string;
  cronTriggerAtStartFieldLabel: string;
  editableConfigurationWorkstationOptionsEmpty: string;
  editableConfigurationWorkstationUnavailablePrefix: string;
  editableConfigurationVisitCountMaxVisitsInvalid: string;
  editableConfigurationVisitCountWorkstationInvalid: (
    workstation: string,
  ) => string;
  editableConfigurationVisitCountWorkstationRequired: string;
  editableConfigurationMatchesFieldsInputKeyRequired: string;
  editableConfigurationInputGuardMultipleGuards: string;
  editableConfigurationInputGuardMatchInputRequired: string;
  editableConfigurationInputGuardMatchInputInvalid: (
    workType: string,
  ) => string;
  editableConfigurationInputGuardMatchInputSelfReference: string;
  editableConfigurationInputGuardParentInputRequired: string;
  editableConfigurationInputGuardParentInputInvalid: (
    workType: string,
  ) => string;
  editableConfigurationInputGuardParentInputSelfReference: string;
  editableConfigurationInputGuardSpawnedByInvalid: (
    workstation: string,
  ) => string;
  matchesFieldsGuardInputKeyFieldLabel: string;
  editableConfigurationGuardSelectorEditorLoading: string;
  editableConfigurationGuardSelectorEditorError: string;
  modelFieldLabel: string;
  notConfiguredValue: string;
  promptFieldLabel: string;
  templateFieldLabel: string;
  workerFieldLabel: string;
  workstationNameFieldLabel: string;
  currentDispatchLabel: string;
  dispatchLabel: string;
  elapsedLabel: string;
  totalRuntimeLabel: string;
  expandAction: string;
  historyRequestCountLabel: (count: number) => string;
  historyRunCountLabel: (count: number) => string;
  historicalRequestsLabel: string;
  historicalRunsLabel: string;
  inputWorkTypesLabel: string;
  kindDefaultValue: string;
  kindLabel: string;
  noWorkstationRequests: string;
  noWorkstationRuns: string;
  openRequestAction: string;
  openRequestDetailsAction: string;
  openNamedWorkItemAction: (workItemLabel: string) => string;
  openWorkItemAction: string;
  outputWorkTypesLabel: string;
  projectedWorkstationRequestSummary: string;
  providerSummary: (provider: string, model?: string | null) => string;
  requestDetailsUnavailable: (dispatchId: string) => string;
  requestHistoryHeading: string;
  requestSelectedAction: string;
  requestStatusStartedAgo: (elapsed: string) => string;
  runnerFieldHelp: (runnerName: string, sourceLabel: string) => string;
  runnerFieldLabel: string;
  runnerInheritanceFactoryLabel: (runnerName: string) => string;
  runnerInheritanceFactoryMissingLabel: string;
  runnerLoadingValue: string;
  runHistoryHeading: string;
  providerSessionLogAction: string;
  providerSessionLogUnavailable: string;
  providerSessionSelectedAction: string;
  providerSessionSelectAction: string;
  providerSessionSelectionUnavailable: string;
  scriptCommandSummary: (command: string) => string;
  selectProviderSessionLabel: (
    sessionLabel: string,
    dispatchId: string,
  ) => string;
  selectRequestLabel: (requestLabel: string, dispatchId: string) => string;
  selectWorkItemLabel: (workItemLabel: string) => string;
  selectWorkstationRequestLabel: (dispatchId: string) => string;
  selectedRequestLabel: (dispatchId: string) => string;
  stationLabel: string;
  summaryHeading: string;
  traceIdLabel: string;
  unknownActiveWorkLabel: string;
  unavailableValue: string;
  unavailableRunnerValue: string;
  unavailableWorkstationKindValue: string;
  unavailableWorkstationTypeValue: string;
  localizeProviderSessionKind: (value: string) => string;
  localizeRunnerSelectionSource: (value: string) => string;
  localizeWorkstationBehavior: (value: string) => string;
  localizeWorkstationKind: (value: string) => string;
  localizeWorkstationType: (value: string) => string;
  localizeWorkstationGuardType: (value: string) => string;
  localizeInputGuardType: (value: string) => string;
  inputGuardMatchInputFieldLabel: string;
  inputGuardParentInputFieldLabel: string;
  inputGuardSpawnedByFieldLabel: string;
  visitCountGuardMaxVisitsFieldLabel: string;
  visitCountGuardWorkstationFieldLabel: string;
  workstationGuardsAddLabel: string;
  workstationGuardsAddPlaceholder: string;
  workstationGuardsEmpty: string;
  workstationGuardsHeading: string;
  workstationGuardsRemoveAction: string;
  workstationInputGuardNoneOption: string;
  workstationInputGuardPeersEmpty: string;
  workstationInputGuardTypeFieldLabel: string;
  workstationInputGuardsEmpty: string;
  workstationInputGuardsHeading: string;
  workstationInputSlotHeading: (workType: string, state: string) => string;
  unknownWorkerTypeValue: string;
  unknownWorkLabel: string;
  workDetailsUnavailable: (dispatchId: string) => string;
  workIdLabel: string;
  workSelectedAction: string;
  workerTypeLabel: string;
  workstationKindLoadingValue: string;
  workstationTypeLabel: string;
  workstationTypeLoadingValue: string;
  selectedRunnerLabel: string;
}
