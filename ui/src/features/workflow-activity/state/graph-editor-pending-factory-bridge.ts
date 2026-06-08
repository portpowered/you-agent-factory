import { create } from "zustand";

import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";

interface GraphEditorPendingFactoryBridgeState {
  pendingFactoryDefinition: CanonicalFactoryDefinition | null;
  setPendingFactoryDefinition: (
    pendingFactoryDefinition: CanonicalFactoryDefinition | null,
  ) => void;
}

export const useGraphEditorPendingFactoryBridge =
  create<GraphEditorPendingFactoryBridgeState>((set) => ({
    pendingFactoryDefinition: null,
    setPendingFactoryDefinition: (pendingFactoryDefinition) => {
      set({ pendingFactoryDefinition });
    },
  }));
