/**
 * Partial mocks for `factory-graph-editor/public` in workflow-activity specs.
 */
import { mock } from "bun:test";

const FACTORY_GRAPH_EDITOR_PUBLIC_MODULE =
  "../src/features/factory-graph-editor/public";

const factoryGraphEditorPublicActual = await import(
  FACTORY_GRAPH_EDITOR_PUBLIC_MODULE,
);

const originalUseEditableFactoryGraph =
  factoryGraphEditorPublicActual.useEditableFactoryGraph;

export const useFactoryGraphDraftStateMock = mock(() => {
  throw new Error("useFactoryGraphDraftStateMock not configured");
});

export const useEditableFactoryGraphMock = mock(
  (
    ...args: Parameters<typeof originalUseEditableFactoryGraph>
  ) => originalUseEditableFactoryGraph(...args),
);

mock.module(FACTORY_GRAPH_EDITOR_PUBLIC_MODULE, () => ({
  ...factoryGraphEditorPublicActual,
  useFactoryGraphDraftState: useFactoryGraphDraftStateMock,
  useEditableFactoryGraph: useEditableFactoryGraphMock,
}));
