import { create } from "zustand";
import type { FactoryGraphBulkSelectionSummary } from "../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import type { FactoryGraphEditorSelectionTarget } from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection";

export type FactoryGraphEditorSelectionBridgeSnapshot = {
  bulkSelectionSummary: FactoryGraphBulkSelectionSummary | null;
  primaryTarget: FactoryGraphEditorSelectionTarget | null;
  selectedEdgeIds: readonly string[];
  selectedNodeIds: readonly string[];
};

interface FactoryGraphEditorSelectionBridgeState {
  selection: FactoryGraphEditorSelectionBridgeSnapshot | null;
  setSelection: (
    selection: FactoryGraphEditorSelectionBridgeSnapshot | null,
  ) => void;
}

export const useFactoryGraphEditorSelectionBridge =
  create<FactoryGraphEditorSelectionBridgeState>((set) => ({
    selection: null,
    setSelection: (selection) => {
      set({ selection });
    },
  }));
