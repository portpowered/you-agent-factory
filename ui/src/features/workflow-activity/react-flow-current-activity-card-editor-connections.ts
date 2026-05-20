import type { Connection } from "@xyflow/react";
import { useCallback, useEffect, useState } from "react";

import type { useFactoryGraphDraftState } from "../factory-graph-editor/factory-graph-draft";
import {
  applyFactoryGraphEdgeAddition,
  buildFactoryGraphConnectionNotice,
  buildFactoryGraphEdgeChangeFromConnection,
  getFactoryGraphConnectionAnchor,
  type FactoryGraphConnectionEndpoint,
} from "../factory-graph-editor/factory-graph-editor-connections";
import type { FactoryGraphEditorTool } from "../factory-graph-editor/factory-graph-editor-controls";

export function useFactoryGraphConnectionController({
  activeTool,
  canInteractWithEditor,
  draftState,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
}) {
  const [connectionNotice, setConnectionNotice] = useState<string | null>(null);
  const [pendingConnectionSource, setPendingConnectionSource] = useState<FactoryGraphConnectionEndpoint | null>(null);
  useEffect(() => {
    if (activeTool !== "connect") {
      setPendingConnectionSource(null);
    }
  }, [activeTool]);
  const commitConnection = useCallback(
    (connection: {
      sourceAnchorId: string;
      sourceNodeId: string;
      targetAnchorId: string;
      targetNodeId: string;
    }) => {
      const edgeChange = buildFactoryGraphEdgeChangeFromConnection(
        draftState.graph,
        connection,
      );
      if (!edgeChange) {
        const sourceNode = draftState.graph.nodes.find(
          (node) => node.id === connection.sourceNodeId,
        );
        const targetNode = draftState.graph.nodes.find(
          (node) => node.id === connection.targetNodeId,
        );
        setConnectionNotice(
          sourceNode && targetNode
            ? buildFactoryGraphConnectionNotice({
                sourceAnchorId: connection.sourceAnchorId,
                sourceNode,
                targetAnchorId: connection.targetAnchorId,
                targetNode,
              })
            : "Choose a compatible source and target anchor before creating a connection.",
        );
        return;
      }
      draftState.updateDraft((currentDraft) =>
        applyFactoryGraphEdgeAddition(currentDraft, draftState.graph, edgeChange),
      );
      setConnectionNotice(null);
      setPendingConnectionSource(null);
    },
    [draftState],
  );
  const handleEditorConnect = useCallback(
    (connection: Connection) => {
      if (
        !canInteractWithEditor ||
        activeTool !== "connect" ||
        !connection.source ||
        !connection.sourceHandle ||
        !connection.target ||
        !connection.targetHandle
      ) {
        return;
      }
      commitConnection({
        sourceAnchorId: connection.sourceHandle,
        sourceNodeId: connection.source,
        targetAnchorId: connection.targetHandle,
        targetNodeId: connection.target,
      });
    },
    [activeTool, canInteractWithEditor, commitConnection],
  );
  const handleConnectionAnchorClick = useCallback(
    (endpoint: FactoryGraphConnectionEndpoint) => {
      if (!canInteractWithEditor || activeTool !== "connect") {
        return;
      }

      const node = draftState.graph.nodes.find((entry) => entry.id === endpoint.nodeId);
      if (!node) {
        return;
      }

      const anchor = getFactoryGraphConnectionAnchor(node.kind, endpoint.anchorId);
      if (!anchor) {
        return;
      }

      if (anchor.role === "source") {
        setPendingConnectionSource((currentSource) =>
          currentSource &&
          currentSource.nodeId === endpoint.nodeId &&
          currentSource.anchorId === endpoint.anchorId
            ? null
            : endpoint,
        );
        setConnectionNotice(null);
        return;
      }

      if (!pendingConnectionSource) {
        setConnectionNotice(
          "Select a source anchor before choosing a target anchor.",
        );
        return;
      }

      commitConnection({
        sourceAnchorId: pendingConnectionSource.anchorId,
        sourceNodeId: pendingConnectionSource.nodeId,
        targetAnchorId: endpoint.anchorId,
        targetNodeId: endpoint.nodeId,
      });
    },
    [activeTool, canInteractWithEditor, commitConnection, draftState.graph.nodes, pendingConnectionSource],
  );
  return { connectionNotice, handleConnectionAnchorClick, handleEditorConnect, pendingConnectionSource, setConnectionNotice };
}
