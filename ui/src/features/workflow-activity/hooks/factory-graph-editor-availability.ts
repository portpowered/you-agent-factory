import type { DashboardTopology } from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-types";

export function findClassifierGraphEditorUnsupportedWorkstationName(
  factoryDefinition: CanonicalFactoryDefinition | null,
): string | undefined {
  const classifierWorkstation = factoryDefinition?.workstations?.find(
    (workstation) =>
      workstation.type === "CLASSIFIER_WORKSTATION" ||
      (workstation.classificationRoutes?.length ?? 0) > 0,
  );

  if (!classifierWorkstation) {
    return undefined;
  }

  return classifierWorkstation.name;
}

export function findClassifierGraphEditorUnsupportedWorkstationNameFromTopology(
  topology: DashboardTopology | null,
): string | undefined {
  const classifierWorkstationNode = topology?.workstation_node_ids
    .map((nodeID) => topology.workstation_nodes_by_id[nodeID])
    .find(
      (workstationNode) =>
        workstationNode?.workstation_kind === "CLASSIFIER_WORKSTATION",
    );

  return classifierWorkstationNode?.workstation_name;
}
