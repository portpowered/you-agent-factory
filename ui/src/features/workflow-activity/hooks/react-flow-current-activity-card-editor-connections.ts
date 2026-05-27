import type { Connection } from "@xyflow/react";
import { useCallback, useEffect, useState } from "react";

import type { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import {
  applyFactoryGraphEdgeAddition,
  buildFactoryGraphConnectionNotice,
  buildFactoryGraphEdgeChangeFromConnection,
  getFactoryGraphConnectionAnchor,
  type FactoryGraphConnectionEndpoint,
} from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: connection controller keeps the React Flow gesture lifecycle together while this story only threads localized notices.
export function useFactoryGraphConnectionController({
  activeTool,
  canInteractWithEditor,
  draftState,
  locale,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  locale?: string;
}) {
  const [connectionNotice, setConnectionNotice] = useState<string | null>(null);
  const [pendingConnectionSource, setPendingConnectionSource] =
    useState<FactoryGraphConnectionEndpoint | null>(null);
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
        const messages = getFactoryGraphEditorMessages(locale);
        setConnectionNotice(
          sourceNode && targetNode
            ? buildFactoryGraphConnectionNotice({
                sourceAnchorId: connection.sourceAnchorId,
                sourceNode,
                targetAnchorId: connection.targetAnchorId,
                targetNode,
                locale,
              })
            : messages.connectionFallbackNotice,
        );
        return;
      }
      draftState.updateDraft((currentDraft) =>
        applyFactoryGraphEdgeAddition(
          currentDraft,
          draftState.graph,
          edgeChange,
        ),
      );
      setConnectionNotice(null);
      setPendingConnectionSource(null);
    },
    [draftState, locale],
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

      const node = draftState.graph.nodes.find(
        (entry) => entry.id === endpoint.nodeId,
      );
      if (!node) {
        return;
      }

      const anchor = getFactoryGraphConnectionAnchor(
        node.kind,
        endpoint.anchorId,
      );
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
    [
      activeTool,
      canInteractWithEditor,
      commitConnection,
      draftState.graph.nodes,
      pendingConnectionSource,
    ],
  );
  return {
    connectionNotice,
    handleConnectionAnchorClick,
    handleEditorConnect,
    pendingConnectionSource,
    setConnectionNotice,
  };
}
