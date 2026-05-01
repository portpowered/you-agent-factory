import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

export interface GraphNodePosition {
  x: number;
  y: number;
}

export type GraphNodePositions = Record<string, GraphNodePosition>;

interface CurrentActivityGraphState {
  positionsByGraphKey: Record<string, GraphNodePositions>;
  setNodePosition: (graphKey: string, nodeId: string, position: GraphNodePosition) => void;
}

export const CURRENT_ACTIVITY_GRAPH_STORAGE_KEY =
  "agent-factory.current-activity.graph-positions.v1";

export const useCurrentActivityGraphStore = create<CurrentActivityGraphState>()(
  persist(
    (set) => ({
      positionsByGraphKey: {},
      setNodePosition: (graphKey, nodeId, position) => {
        set((state) => ({
          positionsByGraphKey: {
            ...state.positionsByGraphKey,
            [graphKey]: {
              ...(state.positionsByGraphKey[graphKey] ?? {}),
              [nodeId]: position,
            },
          },
        }));
      },
    }),
    {
      name: CURRENT_ACTIVITY_GRAPH_STORAGE_KEY,
      partialize: (state) => ({ positionsByGraphKey: state.positionsByGraphKey }),
      storage: createJSONStorage(() => window.localStorage),
    },
  ),
);
