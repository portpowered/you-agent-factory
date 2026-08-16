import type { Connection } from "@xyflow/react";
import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { FactoryGraphEditorTool } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  createFactoryGraphWorkstationResolver,
  type FactoryGraphConnectionEndpoint,
  type FactoryGraphConnectionResolver,
  getFactoryGraphConnectionAnchor,
  resolveFactoryGraphConnectionAnchorContext,
} from "../../factory-graph-editor/lib/editor/factory-graph-editor-connections";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type { WorkflowActivityBentoCardState } from "./workflow-activity-card-state";

type FactoryGraphConnectionCommit = (connection: {
  sourceAnchorId: string;
  sourceNodeId: string;
  targetAnchorId: string;
  targetNodeId: string;
}) => void;

interface FactoryGraphConnectionAnchorHandlerOptions {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  commitConnection: FactoryGraphConnectionCommit;
  draftNodes: EditableFactoryGraphViewModel["draftState"]["graph"]["nodes"];
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  setConnectionNotice: (notice: string | null) => void;
  setPendingConnectionSource: Dispatch<
    SetStateAction<FactoryGraphConnectionEndpoint | null>
  >;
  workstationResolver: FactoryGraphConnectionResolver;
}

function handleFactoryGraphConnectionAnchorClick(
  endpoint: FactoryGraphConnectionEndpoint,
  {
    activeTool,
    canInteractWithEditor,
    commitConnection,
    draftNodes,
    locale,
    hiddenNodeClasses,
    pendingConnectionSource,
    setConnectionNotice,
    setPendingConnectionSource,
    workstationResolver,
  }: FactoryGraphConnectionAnchorHandlerOptions,
): void {
  if (!canInteractWithEditor || activeTool === "delete") {
    return;
  }

  const node = draftNodes.find((entry) => entry.id === endpoint.nodeId);
  if (!node || hiddenNodeClasses.has(node.kind)) {
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

function useFactoryGraphConnectionAnchorHandler({
  activeTool,
  canInteractWithEditor,
  commitConnection,
  draftNodes,
  hiddenNodeClasses,
  locale,
  pendingConnectionSource,
  setConnectionNotice,
  setPendingConnectionSource,
  workstationResolver,
}: FactoryGraphConnectionAnchorHandlerOptions) {
  return useCallback(
    (endpoint: FactoryGraphConnectionEndpoint) => {
      handleFactoryGraphConnectionAnchorClick(endpoint, {
        activeTool,
        canInteractWithEditor,
        commitConnection,
        draftNodes,
        hiddenNodeClasses,
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
      draftNodes,
      hiddenNodeClasses,
      locale,
      pendingConnectionSource,
      setConnectionNotice,
      setPendingConnectionSource,
      workstationResolver,
    ],
  );
}

function useFactoryGraphConnectionIntentState(
  restoredCardState?: Pick<
    WorkflowActivityBentoCardState,
    "connectionNotice" | "pendingConnectionSource"
  >,
) {
  const [connectionNotice, setConnectionNotice] = useState<string | null>(
    () => restoredCardState?.connectionNotice ?? null,
  );
  const [pendingConnectionSource, setPendingConnectionSource] =
    useState<FactoryGraphConnectionEndpoint | null>(
      () => restoredCardState?.pendingConnectionSource ?? null,
    );

  return {
    connectionNotice,
    pendingConnectionSource,
    setConnectionNotice,
    setPendingConnectionSource,
  };
}

export function useFactoryGraphConnectionController({
  activeTool,
  canInteractWithEditor,
  draftState,
  editableGraph,
  hiddenNodeClasses,
  locale,
  restoredCardState,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  draftState: EditableFactoryGraphViewModel["draftState"];
  editableGraph: EditableFactoryGraphViewModel;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string | null;
  restoredCardState?: Pick<
    WorkflowActivityBentoCardState,
    "connectionNotice" | "pendingConnectionSource"
  >;
}) {
  const {
    connectionNotice,
    pendingConnectionSource,
    setConnectionNotice,
    setPendingConnectionSource,
  } = useFactoryGraphConnectionIntentState(restoredCardState);
  const workstationResolver = useMemo(
    () =>
      createFactoryGraphWorkstationResolver(
        draftState.pendingFactoryDefinition?.workstations ??
          draftState.baseDocument?.workstations,
        draftState.pendingFactoryDefinition?.workers ??
          draftState.baseDocument?.workers,
      ),
    [
      draftState.baseDocument?.workers,
      draftState.baseDocument?.workstations,
      draftState.pendingFactoryDefinition?.workers,
      draftState.pendingFactoryDefinition?.workstations,
    ],
  );
  useEffect(() => {
    if (!canInteractWithEditor || activeTool === "delete") {
      setPendingConnectionSource(null);
    }
  }, [activeTool, canInteractWithEditor, setPendingConnectionSource]);
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
    [editableGraph.actions, setConnectionNotice, setPendingConnectionSource],
  );
  const handleEditorConnect = useCallback(
    (connection: Connection) => {
      if (
        !canInteractWithEditor ||
        activeTool === "delete" ||
        !connection.source ||
        !connection.sourceHandle ||
        !connection.target ||
        !connection.targetHandle
      ) {
        return;
      }

      const sourceNode = draftState.graph.nodes.find(
        (entry) => entry.id === connection.source,
      );
      const targetNode = draftState.graph.nodes.find(
        (entry) => entry.id === connection.target,
      );
      if (
        (sourceNode && hiddenNodeClasses.has(sourceNode.kind)) ||
        (targetNode && hiddenNodeClasses.has(targetNode.kind))
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
    [
      activeTool,
      canInteractWithEditor,
      commitConnection,
      draftState.graph.nodes,
      hiddenNodeClasses,
    ],
  );
  const handleConnectionAnchorClick = useFactoryGraphConnectionAnchorHandler({
    activeTool,
    canInteractWithEditor,
    commitConnection,
    draftNodes: draftState.graph.nodes,
    hiddenNodeClasses,
    locale,
    pendingConnectionSource,
    setConnectionNotice,
    setPendingConnectionSource,
    workstationResolver,
  });
  return {
    connectionNotice,
    handleConnectionAnchorClick,
    handleEditorConnect,
    pendingConnectionSource,
    setConnectionNotice,
  };
}
