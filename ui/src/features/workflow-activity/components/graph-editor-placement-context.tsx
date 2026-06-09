import type { ReactFlowInstance } from "@xyflow/react";
import {
  createContext,
  type MutableRefObject,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import type { FactoryGraphAddEntityDraft } from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import type { CurrentActivityNode } from "../../flowchart/public";
import {
  factoryGraphNodeIdForAddEntityDraft,
  resolveInitialPlacementTopLeft,
  viewportCenterInFlowCoordinates,
} from "../lib/graph-editor-add-node-placement";

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
  moveLayoutNode,
  nodes,
}: {
  flowContainerRef: MutableRefObject<HTMLElement | null>;
  flowInstanceRef: MutableRefObject<ReactFlowInstance | null>;
  moveLayoutNode?: (nodeId: string, position: { x: number; y: number }) => void;
  nodes: readonly CurrentActivityNode[];
}) {
  const apiRef = useContext(GraphEditorPlacementContext);
  const [pendingPlacement, setPendingPlacement] = useState<{
    draft: FactoryGraphAddEntityDraft;
  } | null>(null);
  useLayoutEffect(() => {
    if (!apiRef) {
      return;
    }

    apiRef.current = {
      placeAddedNode: (draft) => {
        setPendingPlacement({
          draft,
        });
      },
    };
  }, [apiRef]);

  useEffect(() => {
    if (!pendingPlacement) {
      return;
    }

    const flowInstance = flowInstanceRef.current;
    const flowContainer = flowContainerRef.current;
    if (!flowInstance || !flowContainer) {
      return;
    }

    const topLeft = resolveInitialPlacementTopLeft({
      draft: pendingPlacement.draft,
      nodes,
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
    }
    setPendingPlacement(null);
  }, [
    flowContainerRef,
    flowInstanceRef,
    moveLayoutNode,
    nodes,
    pendingPlacement,
  ]);

  return null;
}
