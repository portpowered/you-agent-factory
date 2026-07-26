export {
  DashboardSessionTestProvider,
  type DashboardSessionTestProviderProps,
} from "./dashboard-session-test-provider";
export {
  renderWithDashboardSessionTest,
  wrapWithDashboardSessionTest,
} from "./dashboard-session-test-utils";
export {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
  createHookTestGraphEditorDraftState,
  createMockEditableFactoryGraph,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
  type MockEditableFactoryGraphHooks,
  type MockGraphEditorDraftState,
  wireMockEditableFactoryGraph,
  workerDenseFactoryDefinitionDocument,
} from "./graph-editor-harness";
export {
  assertStrictConsoleClean,
  type ConsoleAllowlistEntry,
  type ConsoleLevel,
  installStrictConsoleGuard,
  type StrictConsoleGuardOptions,
  useStrictConsoleGuard,
  withStrictConsole,
} from "./strict-console-guard";
