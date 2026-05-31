/**
 * Partial mocks for factory-graph-editor hooks in workflow-activity specs.
 */
import { mock } from "bun:test";

const FACTORY_GRAPH_DRAFT_HOOK_MODULE =
  "../src/features/factory-graph-editor/hooks/factory-graph-draft-hook";
const EDITABLE_FACTORY_GRAPH_HOOK_MODULE =
  "../src/features/factory-graph-editor/hooks/use-editable-factory-graph";

const factoryGraphDraftHookActual = await import(
  FACTORY_GRAPH_DRAFT_HOOK_MODULE,
);
const editableFactoryGraphActual = await import(
  EDITABLE_FACTORY_GRAPH_HOOK_MODULE,
);

const originalUseEditableFactoryGraph =
  editableFactoryGraphActual.useEditableFactoryGraph;

export const useFactoryGraphDraftStateMock = mock(() => {
  throw new Error("useFactoryGraphDraftStateMock not configured");
});

export const useEditableFactoryGraphMock = mock(
  (
    ...args: Parameters<typeof originalUseEditableFactoryGraph>
  ) => originalUseEditableFactoryGraph(...args),
);

mock.module(FACTORY_GRAPH_DRAFT_HOOK_MODULE, () => ({
  ...factoryGraphDraftHookActual,
  useFactoryGraphDraftState: useFactoryGraphDraftStateMock,
}));

mock.module(EDITABLE_FACTORY_GRAPH_HOOK_MODULE, () => ({
  ...editableFactoryGraphActual,
  useEditableFactoryGraph: useEditableFactoryGraphMock,
}));
