import type { ReactFlowInstance } from "@xyflow/react";
import {
  createContext,
  type MutableRefObject,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/factory-graph-editor-additions";
import type { CurrentActivityNode } from "../../flowchart/public";
import {
  factoryGraphNodeIdForAddEntityDraft,
  resolveInitialPlacementTopLeft,
  viewportCenterInFlowCoordinates,
} from "../lib/graph-editor-add-node-placement";
import { graphKeyAfterAddingNode } from "../lib/react-flow-current-activity-card-keys";
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
  moveLayoutNode,
  nodes,
  setStoredNodePosition,
  storedNodePositions,
}: {
  flowContainerRef: MutableRefObject<HTMLElement | null>;
  flowInstanceRef: MutableRefObject<ReactFlowInstance | null>;
  graphKey: string;
  moveLayoutNode?: (nodeId: string, position: { x: number; y: number }) => void;
  nodes: readonly CurrentActivityNode[];
  setStoredNodePosition: (
    graphKey: string,
    nodeId: string,
    position: { x: number; y: number },
  ) => void;
  storedNodePositions: GraphNodePositions;
}) {
  const apiRef = useContext(GraphEditorPlacementContext);
  const [pendingPlacement, setPendingPlacement] = useState<{
    draft: FactoryGraphAddEntityDraft;
    storageGraphKey: string;
  } | null>(null);
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
        const nodeId = factoryGraphNodeIdForAddEntityDraft(draft);
        setPendingPlacement({
          draft,
          storageGraphKey: graphKeyAfterAddingNode(graphKey, nodeId),
        });
      },
    };
  }, [apiRef, graphKey]);

  useEffect(() => {
    if (!pendingPlacement || !placementInput.graphKey) {
      return;
    }

    const flowInstance = flowInstanceRef.current;
    const flowContainer = flowContainerRef.current;
    if (!flowInstance || !flowContainer) {
      return;
    }

    const topLeft = resolveInitialPlacementTopLeft({
      draft: pendingPlacement.draft,
      nodes: placementInput.nodes,
      storedPositions: placementInput.storedNodePositions,
      viewportCenter: viewportCenterInFlowCoordinates(
        flowInstance,
        flowContainer,
      ),
    });
    if (!topLeft) {
      setPendingPlacement(null);
      return;
    }

    const nodeId = factoryGraphNodeIdForAddEntityDraft(pendingPlacement.draft);
    if (moveLayoutNode) {
      moveLayoutNode(nodeId, topLeft);
    } else {
      placementInput.setStoredNodePosition(
        pendingPlacement.storageGraphKey,
        nodeId,
        topLeft,
      );
    }
    setPendingPlacement(null);
  }, [
    flowContainerRef,
    flowInstanceRef,
    moveLayoutNode,
    pendingPlacement,
    placementInput,
  ]);

  return null;
}
