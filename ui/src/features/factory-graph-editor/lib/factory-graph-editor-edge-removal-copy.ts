import type {
  FactoryGraphEdge,
  FactoryGraphNodeKey,
} from "./factory-graph-draft-types";
import { getFactoryGraphEditorMessages } from "../messages/editor";

export function buildEdgeRemovalDescription(
  edge: FactoryGraphEdge,
  locale?: string | null,
) {
  if (edge.kind === "work-state-visibility-bypass") {
    return "";
  }
  const messages = getFactoryGraphEditorMessages(locale);
  return messages.removalEdgeDescription(
    edge.kind,
    describeNodeLabel(edge.source),
    describeNodeLabel(edge.target),
  );
}

export function describeEdgeLabel(
  edge: FactoryGraphEdge,
  locale?: string | null,
) {
  if (edge.kind === "work-state-visibility-bypass") {
    return "";
  }
  const messages = getFactoryGraphEditorMessages(locale);
  return messages.removalEdgeLabel(edge.kind, describeNodeLabel(edge.source));
}

function describeNodeLabel(key: FactoryGraphNodeKey) {
  return key.kind === "work-state"
    ? `${key.workTypeName}:${key.stateName}`
    : key.name;
}
