import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import { migrateWorkStateGraphLayoutPositions } from "../lib/migrate-work-state-graph-layout-positions";

export interface GraphNodePosition {
  x: number;
  y: number;
}

export type GraphNodePositions = Record<string, GraphNodePosition>;

export interface MigrateWorkStateNodePositionsInput {
  nextStateName: string;
  previousStateName: string;
  workTypeName: string;
}

interface CurrentActivityGraphState {
  migrateWorkStateNodePositions: (
    input: MigrateWorkStateNodePositionsInput,
  ) => void;
  positionsByGraphKey: Record<string, GraphNodePositions>;
  setNodePosition: (
    graphKey: string,
    nodeId: string,
    position: GraphNodePosition,
  ) => void;
}

export const CURRENT_ACTIVITY_GRAPH_STORAGE_KEY =
  "agent-factory.current-activity.graph-positions.v1";

export const useCurrentActivityGraphStore = create<CurrentActivityGraphState>()(
  persist(
    (set) => ({
      migrateWorkStateNodePositions: (input) => {
        set((state) => ({
          positionsByGraphKey: migrateWorkStateGraphLayoutPositions({
            ...input,
            positionsByGraphKey: state.positionsByGraphKey,
          }),
        }));
      },
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
      partialize: (state) => ({
        positionsByGraphKey: state.positionsByGraphKey,
      }),
      storage: createJSONStorage(() => window.localStorage),
    },
  ),
);
