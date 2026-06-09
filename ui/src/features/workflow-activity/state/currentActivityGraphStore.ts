import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import {
  bridgeGraphLayoutPositions,
  clearStoredNodePositionsForNodeIds,
} from "../lib/bridge-graph-layout-positions";
import { migrateWorkStateGraphLayoutPositions } from "../lib/migrate-work-state-graph-layout-positions";

export interface GraphNodePosition {
  x: number;
  y: number;
}

export interface GraphViewportPosition {
  x: number;
  y: number;
  zoom: number;
}

export type GraphNodePositions = Record<string, GraphNodePosition>;

export interface MigrateWorkStateNodePositionsInput {
  nextStateName: string;
  previousStateName: string;
  workTypeName: string;
}

interface CurrentActivityGraphState {
  bridgePositionsToGraphKey: (
    graphKey: string,
    nodeIds: readonly string[],
  ) => void;
  clearNodePositions: (nodeIds: readonly string[]) => void;
  clearViewport: (graphKey: string) => void;
  migrateWorkStateNodePositions: (
    input: MigrateWorkStateNodePositionsInput,
  ) => void;
  positionsByGraphKey: Record<string, GraphNodePositions>;
  viewportByGraphKey: Record<string, GraphViewportPosition>;
  setNodePosition: (
    graphKey: string,
    nodeId: string,
    position: GraphNodePosition,
  ) => void;
  setViewport: (graphKey: string, viewport: GraphViewportPosition) => void;
}

export const CURRENT_ACTIVITY_GRAPH_STORAGE_KEY =
  "agent-factory.current-activity.graph-positions.v1";

export const useCurrentActivityGraphStore = create<CurrentActivityGraphState>()(
  persist(
    (set) => ({
      bridgePositionsToGraphKey: (graphKey, nodeIds) => {
        set((state) => {
          const nextPositionsByGraphKey = bridgeGraphLayoutPositions({
            nodeIds,
            positionsByGraphKey: state.positionsByGraphKey,
            targetGraphKey: graphKey,
          });
          if (!nextPositionsByGraphKey) {
            return state;
          }

          return { positionsByGraphKey: nextPositionsByGraphKey };
        });
      },
      clearNodePositions: (nodeIds) => {
        set((state) => {
          const nextPositionsByGraphKey = clearStoredNodePositionsForNodeIds(
            state.positionsByGraphKey,
            nodeIds,
          );
          if (!nextPositionsByGraphKey) {
            return state;
          }

          return { positionsByGraphKey: nextPositionsByGraphKey };
        });
      },
      clearViewport: (graphKey) => {
        set((state) => {
          if (!(graphKey in state.viewportByGraphKey)) {
            return state;
          }

          const nextViewportByGraphKey = { ...state.viewportByGraphKey };
          delete nextViewportByGraphKey[graphKey];
          return { viewportByGraphKey: nextViewportByGraphKey };
        });
      },
      migrateWorkStateNodePositions: (input) => {
        set((state) => ({
          positionsByGraphKey: migrateWorkStateGraphLayoutPositions({
            ...input,
            positionsByGraphKey: state.positionsByGraphKey,
          }),
        }));
      },
      positionsByGraphKey: {},
      viewportByGraphKey: {},
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
      setViewport: (graphKey, viewport) => {
        set((state) => ({
          viewportByGraphKey: {
            ...state.viewportByGraphKey,
            [graphKey]: viewport,
          },
        }));
      },
    }),
    {
      name: CURRENT_ACTIVITY_GRAPH_STORAGE_KEY,
      partialize: (state) => ({
        positionsByGraphKey: state.positionsByGraphKey,
        viewportByGraphKey: state.viewportByGraphKey,
      }),
      storage: createJSONStorage(() => window.localStorage),
    },
  ),
);
