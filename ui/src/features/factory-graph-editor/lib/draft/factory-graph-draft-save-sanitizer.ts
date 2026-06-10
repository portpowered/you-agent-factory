import {
  collectFactoryGraphResourceNames,
  isInternalResourceAvailabilityGraphEdgeId,
  isInternalResourceAvailabilityGraphNodeId,
  isInternalResourceAvailabilityIO,
  isInternalResourceAvailabilityWorkstation,
  isInternalResourceAvailabilityWorkType,
} from "./factory-graph-draft-internal-resources";
import {
  isInternalSystemTimeGraphEdgeId,
  isInternalSystemTimeGraphNodeId,
  isInternalSystemTimeIO,
  isInternalSystemTimeWorkState,
  isInternalSystemTimeWorkstation,
  isInternalSystemTimeWorkType,
} from "./factory-graph-draft-internal-system-time";
import type {
  CanonicalFactoryDefinition,
  FactoryWorkstation,
  FactoryWorkstationIO,
} from "./factory-graph-draft-types";

type FactoryWorkstationIORoute =
  | FactoryWorkstationIO
  | FactoryWorkstationIO[]
  | null
  | undefined;

export function removeInternalSystemTimeFactoryGraph(
  factoryDefinition: CanonicalFactoryDefinition,
): CanonicalFactoryDefinition {
  const nextFactoryDefinition = structuredClone(factoryDefinition);
  const resourceNames = collectFactoryGraphResourceNames(nextFactoryDefinition);
  nextFactoryDefinition.workTypes = (nextFactoryDefinition.workTypes ?? [])
    .filter(
      (workType) =>
        !isInternalSystemTimeWorkType(workType) &&
        !isInternalResourceAvailabilityWorkType(workType, resourceNames),
    )
    .map((workType) => ({
      ...workType,
      states: workType.states.filter(
        (state) =>
          !isInternalSystemTimeWorkState(state, workType.id ?? workType.name),
      ),
    }));
  nextFactoryDefinition.workstations = (
    nextFactoryDefinition.workstations ?? []
  )
    .filter(
      (workstation) =>
        !isInternalSystemTimeWorkstation(workstation) &&
        !isInternalResourceAvailabilityWorkstation(workstation, resourceNames),
    )
    .map((workstation) =>
      removeInternalWorkstationRoutes(workstation, resourceNames),
    );

  if (nextFactoryDefinition.layout) {
    nextFactoryDefinition.layout = {
      ...nextFactoryDefinition.layout,
      edges: nextFactoryDefinition.layout.edges?.filter(
        (edge) =>
          !isInternalSystemTimeGraphEdgeId(edge.id) &&
          !isInternalResourceAvailabilityGraphEdgeId(edge.id, resourceNames),
      ),
      nodes: nextFactoryDefinition.layout.nodes?.filter(
        (node) =>
          !isInternalSystemTimeGraphNodeId(node.id) &&
          !isInternalResourceAvailabilityGraphNodeId(node.id, resourceNames),
      ),
    };
  }

  return nextFactoryDefinition;
}

function removeInternalWorkstationRoutes(
  workstation: FactoryWorkstation,
  resourceNames: ReadonlySet<string>,
): FactoryWorkstation {
  const nextWorkstation = structuredClone(workstation);
  nextWorkstation.inputs = nextWorkstation.inputs.filter(
    (route) =>
      !isInternalSystemTimeIO(route) &&
      !isInternalResourceAvailabilityIO(route, resourceNames),
  );
  assignOptionalProperty(
    nextWorkstation,
    "outputs",
    removeInternalOptionalRoutes(nextWorkstation.outputs, resourceNames),
  );
  assignOptionalProperty(
    nextWorkstation,
    "onContinue",
    removeInternalOptionalRoutes(nextWorkstation.onContinue, resourceNames),
  );
  assignOptionalProperty(
    nextWorkstation,
    "onFailure",
    removeInternalOptionalRoutes(nextWorkstation.onFailure, resourceNames),
  );
  assignOptionalProperty(
    nextWorkstation,
    "onRejection",
    removeInternalOptionalRoutes(nextWorkstation.onRejection, resourceNames),
  );
  return nextWorkstation;
}

function removeInternalOptionalRoutes(
  routes: FactoryWorkstationIORoute,
  resourceNames: ReadonlySet<string>,
): FactoryWorkstationIO[] | undefined {
  const nextRoutes = workstationIOEntries(routes).filter(
    (route) =>
      !isInternalSystemTimeIO(route) &&
      !isInternalResourceAvailabilityIO(route, resourceNames),
  );
  return nextRoutes.length > 0 ? nextRoutes : undefined;
}

function workstationIOEntries(
  items: FactoryWorkstationIORoute,
): FactoryWorkstationIO[] {
  if (!items) {
    return [];
  }
  return Array.isArray(items) ? items : [items];
}

function assignOptionalProperty<T extends object, K extends keyof T>(
  target: T,
  key: K,
  value: T[K] | undefined,
): void {
  if (value === undefined) {
    delete target[key];
    return;
  }

  target[key] = value;
}
