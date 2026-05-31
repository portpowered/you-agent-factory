import type { ReactFlowInstance } from "@xyflow/react";
import {
  createContext,
  type MutableRefObject,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
} from "react";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import type { CurrentActivityNode } from "../../flowchart/public";
import {
  factoryGraphNodeIdForAddEntityDraft,
  resolveInitialPlacementTopLeft,
  viewportCenterInFlowCoordinates,
} from "../lib/graph-editor-add-node-placement";
import type { GraphNodePositions } from "../state/currentActivityGraphStore";

export interface GraphEditorPlacementApi {
  placeAddedNode: (draft: FactoryGraphAddEntityDraft) => void;
}

const GraphEditorPlacementContext =
  createContext<MutableRefObject<GraphEditorPlacementApi> | null>(null);

export function useGraphEditorPlaceAddedNode():
  | ((draft: FactoryGraphAddEntityDraft) => void)
  | undefined {
  const apiRef = useContext(GraphEditorPlacementContext);
  return useCallback(
    (draft: FactoryGraphAddEntityDraft) => {
      apiRef?.current.placeAddedNode(draft);
    },
    [apiRef],
  );
}

export function GraphEditorPlacementProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const apiRef = useRef<GraphEditorPlacementApi>({
    placeAddedNode: () => {},
  });

  return (
    <GraphEditorPlacementContext.Provider value={apiRef}>
      {children}
    </GraphEditorPlacementContext.Provider>
  );
}

export function GraphEditorPlacementRegistrar({
  flowContainerRef,
  flowInstanceRef,
  graphKey,
  nodes,
  setStoredNodePosition,
  storedNodePositions,
}: {
  flowContainerRef: MutableRefObject<HTMLElement | null>;
  flowInstanceRef: MutableRefObject<ReactFlowInstance | null>;
  graphKey: string;
  nodes: readonly CurrentActivityNode[];
  setStoredNodePosition: (
    graphKey: string,
    nodeId: string,
    position: { x: number; y: number },
  ) => void;
  storedNodePositions: GraphNodePositions;
}) {
  const apiRef = useContext(GraphEditorPlacementContext);
  const placementInput = useMemo(
    () => ({
      graphKey,
      nodes,
      setStoredNodePosition,
      storedNodePositions,
    }),
    [graphKey, nodes, setStoredNodePosition, storedNodePositions],
  );

  useLayoutEffect(() => {
    if (!apiRef) {
      return;
    }

    apiRef.current = {
      placeAddedNode: (draft) => {
        if (!placementInput.graphKey) {
          return;
        }

        const flowInstance = flowInstanceRef.current;
        const flowContainer = flowContainerRef.current;
        if (!flowInstance || !flowContainer) {
          return;
        }

        const topLeft = resolveInitialPlacementTopLeft({
          draft,
          nodes: placementInput.nodes,
          storedPositions: placementInput.storedNodePositions,
          viewportCenter: viewportCenterInFlowCoordinates(
            flowInstance,
            flowContainer,
          ),
        });
        if (!topLeft) {
          return;
        }

        placementInput.setStoredNodePosition(
          placementInput.graphKey,
          factoryGraphNodeIdForAddEntityDraft(draft),
          topLeft,
        );
      },
    };
  }, [apiRef, flowContainerRef, flowInstanceRef, placementInput]);

  return null;
}
