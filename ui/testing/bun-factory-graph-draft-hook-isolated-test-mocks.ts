/**
 * Isolated draft-hook stub for use-editable-factory-graph view-model specs.
 * Import only from specs that intentionally replace useFactoryGraphDraftState.
 */
import { mock } from "bun:test";

import type { MockGraphEditorDraftState } from "../src/testing/graph-editor-harness";

const FACTORY_GRAPH_DRAFT_HOOK_MODULE =
  "../src/features/factory-graph-editor/hooks/factory-graph-draft-hook";

const factoryGraphDraftHookActual = await import(FACTORY_GRAPH_DRAFT_HOOK_MODULE);

export const factoryGraphDraftHookTestState: {
  draftState: MockGraphEditorDraftState;
} = {
  draftState: {} as MockGraphEditorDraftState,
};

export const useFactoryGraphDraftStateIsolatedMock = mock(
  () => factoryGraphDraftHookTestState.draftState,
);

mock.module(FACTORY_GRAPH_DRAFT_HOOK_MODULE, () => ({
  ...factoryGraphDraftHookActual,
  useFactoryGraphDraftState: useFactoryGraphDraftStateIsolatedMock,
}));
