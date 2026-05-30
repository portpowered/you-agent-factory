import type { Connection } from "@xyflow/react";
import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  createFactoryGraphWorkstationResolver,
  type FactoryGraphConnectionResolver,
  resolveFactoryGraphConnectionAnchorContext,
} from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import {
  type FactoryGraphConnectionEndpoint,
  getFactoryGraphConnectionAnchor,
} from "../../factory-graph-editor/lib/factory-graph-editor-connections";

type FactoryGraphConnectionCommit = (connection: {
  sourceAnchorId: string;
  sourceNodeId: string;
  targetAnchorId: string;
  targetNodeId: string;
}) => void;

function handleFactoryGraphConnectionAnchorClick(
  endpoint: FactoryGraphConnectionEndpoint,
  {
    activeTool,
    canInteractWithEditor,
    commitConnection,
    draftNodes,
    locale,
    pendingConnectionSource,
    setConnectionNotice,
    setPendingConnectionSource,
    workstationResolver,
  }: {
    activeTool: FactoryGraphEditorTool;
    canInteractWithEditor: boolean;
    commitConnection: FactoryGraphConnectionCommit;
    draftNodes: EditableFactoryGraphViewModel["draftState"]["graph"]["nodes"];
    locale?: string | null;
    pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
    setConnectionNotice: (notice: string | null) => void;
    setPendingConnectionSource: Dispatch<
      SetStateAction<FactoryGraphConnectionEndpoint | null>
    >;
    workstationResolver: FactoryGraphConnectionResolver;
  },
): void {
  if (!canInteractWithEditor || activeTool !== "connect") {
    return;
  }

  const node = draftNodes.find((entry) => entry.id === endpoint.nodeId);
  if (!node) {
    return;
  }

  const anchorContext = resolveFactoryGraphConnectionAnchorContext(
    node,
    workstationResolver,
  );
  const anchor = getFactoryGraphConnectionAnchor(
    node.kind,
    endpoint.anchorId,
    anchorContext,
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
    const messages = getFactoryGraphEditorMessages(locale);
    setConnectionNotice(messages.connectionSelectSourceNotice);
    return;
  }

  commitConnection({
    sourceAnchorId: pendingConnectionSource.anchorId,
    sourceNodeId: pendingConnectionSource.nodeId,
    targetAnchorId: endpoint.anchorId,
    targetNodeId: endpoint.nodeId,
  });
}

export function useFactoryGraphConnectionController({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  locale,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  locale?: string | null;
}) {
  const [connectionNotice, setConnectionNotice] = useState<string | null>(null);
  const [pendingConnectionSource, setPendingConnectionSource] =
    useState<FactoryGraphConnectionEndpoint | null>(null);
  const workstationResolver = useMemo(
    () =>
      createFactoryGraphWorkstationResolver(
        draftState.pendingFactoryDefinition?.workstations ??
          draftState.baseDocument?.workstations,
      ),
    [
      draftState.baseDocument?.workstations,
      draftState.pendingFactoryDefinition?.workstations,
    ],
  );
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
      const result = editableGraph.actions.connectNodes(connection);
      if (!result.ok) {
        setConnectionNotice(result.message);
        return;
      }
      setConnectionNotice(null);
      setPendingConnectionSource(null);
    },
    [editableGraph.actions],
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
      handleFactoryGraphConnectionAnchorClick(endpoint, {
        activeTool,
        canInteractWithEditor,
        commitConnection,
        draftNodes: draftState.graph.nodes,
        locale,
        pendingConnectionSource,
        setConnectionNotice,
        setPendingConnectionSource,
        workstationResolver,
      });
    },
    [
      activeTool,
      canInteractWithEditor,
      commitConnection,
      draftState.graph.nodes,
      locale,
      pendingConnectionSource,
      workstationResolver,
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
