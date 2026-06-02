import { create } from "zustand";

export interface FactoryGraphTopologyEditorBridgeHandlers {
  blockedRemovalReason: string | null;
  canInteractWithEditor: boolean;
  editorMode: boolean;
  requestNodeRemoval: (nodeId: string) => void;
}

interface FactoryGraphTopologyEditorBridgeState {
  graphDraftHasPendingChanges: boolean;
  handlers: FactoryGraphTopologyEditorBridgeHandlers | null;
  setGraphDraftHasPendingChanges: (
    graphDraftHasPendingChanges: boolean,
  ) => void;
  setHandlers: (
    handlers: FactoryGraphTopologyEditorBridgeHandlers | null,
  ) => void;
}

export const useFactoryGraphTopologyEditorBridge =
  create<FactoryGraphTopologyEditorBridgeState>((set) => ({
    graphDraftHasPendingChanges: false,
    handlers: null,
    setGraphDraftHasPendingChanges: (graphDraftHasPendingChanges) => {
      set({ graphDraftHasPendingChanges });
    },
    setHandlers: (handlers) => {
      set({ handlers });
    },
  }));

export function readGraphDraftHasPendingChanges(): boolean {
  return useFactoryGraphTopologyEditorBridge.getState()
    .graphDraftHasPendingChanges;
}
