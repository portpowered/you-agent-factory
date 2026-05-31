import { create } from "zustand";

export interface FactoryGraphTopologyEditorBridgeHandlers {
  blockedRemovalReason: string | null;
  canInteractWithEditor: boolean;
  editorMode: boolean;
  requestNodeRemoval: (nodeId: string) => void;
}

interface FactoryGraphTopologyEditorBridgeState {
  handlers: FactoryGraphTopologyEditorBridgeHandlers | null;
  setHandlers: (handlers: FactoryGraphTopologyEditorBridgeHandlers | null) => void;
}

export const useFactoryGraphTopologyEditorBridge =
  create<FactoryGraphTopologyEditorBridgeState>((set) => ({
    handlers: null,
    setHandlers: (handlers) => {
      set({ handlers });
    },
  }));
