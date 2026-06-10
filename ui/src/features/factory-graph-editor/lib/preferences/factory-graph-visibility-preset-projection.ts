import type { FactoryGraphEditorVisibilityPreset } from "../../components/controls/factory-graph-editor-controls";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";

function nodeVisibleForVisibilityPreset(
  kind: FactoryGraphNodeKind,
  preset: FactoryGraphEditorVisibilityPreset,
): boolean {
  switch (preset) {
    case "all":
      return true;
    case "workflow":
      return (
        kind === "doc" ||
        kind === "workstation" ||
        kind === "work-type" ||
        kind === "work-state"
      );
    case "execution":
      return kind === "workstation" || kind === "work-state";
    case "infrastructure":
      return (
        kind === "doc" ||
        kind === "resource" ||
        kind === "worker" ||
        kind === "workstation"
      );
  }
}

function edgeVisibleForVisibilityPreset(
  edgeKind: FactoryGraphTopology["edges"][number]["kind"],
  preset: FactoryGraphEditorVisibilityPreset,
): boolean {
  switch (preset) {
    case "all":
      return true;
    case "workflow":
      return (
        edgeKind === "work-type-state" ||
        edgeKind === "workstation-input" ||
        edgeKind === "workstation-output"
      );
    case "execution":
      return (
        edgeKind === "work-type-state" ||
        edgeKind === "workstation-input" ||
        edgeKind === "workstation-output" ||
        edgeKind === "workstation-on-continue" ||
        edgeKind === "workstation-on-failure" ||
        edgeKind === "workstation-on-rejection"
      );
    case "infrastructure":
      return (
        edgeKind === "worker-assignment" ||
        edgeKind === "worker-resource" ||
        edgeKind === "workstation-resource"
      );
  }
}

export function projectFactoryGraphByVisibilityPreset(
  topology: FactoryGraphTopology,
  preset: FactoryGraphEditorVisibilityPreset,
): FactoryGraphTopology {
  if (preset === "all") {
    return topology;
  }

  const nodes = topology.nodes.filter((node) =>
    nodeVisibleForVisibilityPreset(node.kind, preset),
  );
  const visibleNodeIds = new Set(nodes.map((node) => node.id));
  const edges = topology.edges.filter(
    (edge) =>
      visibleNodeIds.has(edge.sourceId) &&
      visibleNodeIds.has(edge.targetId) &&
      edgeVisibleForVisibilityPreset(edge.kind, preset),
  );

  return { edges, nodes };
}
