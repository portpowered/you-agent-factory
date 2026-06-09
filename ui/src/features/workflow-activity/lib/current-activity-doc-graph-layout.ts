import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { GraphLayout, PositionedDocNode } from "../../flowchart/lib/layout";
import { listFactoryBundledDocs } from "./factory-bundled-docs";

export const CURRENT_ACTIVITY_DOC_NODE_WIDTH = 168;
export const CURRENT_ACTIVITY_DOC_NODE_HEIGHT = 86;
const DOC_COLUMN_GAP = 48;
const DOC_ROW_GAP = 24;

export function mergeDocNodesIntoGraphLayout(
  topologyLayout: GraphLayout,
  factory: CanonicalFactoryDefinition | null | undefined,
): GraphLayout {
  const docs = listFactoryBundledDocs(factory);
  if (docs.length === 0) {
    return topologyLayout;
  }

  const baseX =
    topologyLayout.width > 0
      ? topologyLayout.width + DOC_COLUMN_GAP
      : 0;
  const docNodes: PositionedDocNode[] = docs.map((doc, index) => ({
    column: 0,
    displayLabel: doc.displayLabel,
    height: CURRENT_ACTIVITY_DOC_NODE_HEIGHT,
    nodeId: doc.nodeId,
    nodeKind: "doc",
    row: index,
    targetPath: doc.targetPath,
    width: CURRENT_ACTIVITY_DOC_NODE_WIDTH,
    x: baseX,
    y: index * (CURRENT_ACTIVITY_DOC_NODE_HEIGHT + DOC_ROW_GAP),
  }));

  const docColumnWidth = baseX + CURRENT_ACTIVITY_DOC_NODE_WIDTH;
  const docColumnHeight =
    docs.length * CURRENT_ACTIVITY_DOC_NODE_HEIGHT +
    Math.max(0, docs.length - 1) * DOC_ROW_GAP;

  return {
    edges: topologyLayout.edges,
    height: Math.max(topologyLayout.height, docColumnHeight),
    nodes: [...topologyLayout.nodes, ...docNodes],
    width: Math.max(topologyLayout.width, docColumnWidth),
  };
}
