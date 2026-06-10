import { getFactoryGraphEditorMessages } from "../../messages/editor";
import {
  collectFactoryGraphResourceNames,
  isInternalResourceAvailabilityIO,
} from "./factory-graph-draft-internal-resources";
import {
  isInternalSystemTimeIO,
  isInternalSystemTimeWorkState,
  isInternalSystemTimeWorkstation,
  isInternalSystemTimeWorkType,
} from "./factory-graph-draft-internal-system-time";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraftEdgeChange,
  FactoryGraphDraftValidationError,
  FactoryWorkstation,
  FactoryWorkstationIO,
} from "./factory-graph-draft-types";
import { nodeKeyId } from "./factory-graph-draft-types";

type WorkstationIORouteField =
  | "inputs"
  | "onContinue"
  | "onFailure"
  | "onRejection"
  | "outputs";

type FactoryWorkstationIORoute =
  | FactoryWorkstationIO
  | FactoryWorkstationIO[]
  | null
  | undefined;

const ROUTE_EDGE_KIND_BY_FIELD = {
  inputs: "workstation-input",
  onContinue: "workstation-on-continue",
  onFailure: "workstation-on-failure",
  onRejection: "workstation-on-rejection",
  outputs: "workstation-output",
} as const satisfies Record<
  WorkstationIORouteField,
  FactoryGraphDraftEdgeChange["kind"]
>;

export function validateFinalWorkstationRoutes(
  pendingFactoryDefinition: CanonicalFactoryDefinition,
  errors: FactoryGraphDraftValidationError[],
  locale?: string | null,
) {
  const validPlaces = collectFactoryGraphValidPlaces(pendingFactoryDefinition);
  const resourceNames = collectFactoryGraphResourceNames(
    pendingFactoryDefinition,
  );

  for (const workstation of pendingFactoryDefinition.workstations ?? []) {
    if (isInternalSystemTimeWorkstation(workstation)) {
      continue;
    }

    validateWorkstationIORoutes(
      workstation,
      "inputs",
      workstation.inputs,
      validPlaces,
      resourceNames,
      errors,
      locale,
    );
    validateWorkstationIORoutes(
      workstation,
      "outputs",
      workstation.outputs,
      validPlaces,
      resourceNames,
      errors,
      locale,
    );
    validateWorkstationIORoutes(
      workstation,
      "onContinue",
      workstation.onContinue,
      validPlaces,
      resourceNames,
      errors,
      locale,
    );
    validateWorkstationIORoutes(
      workstation,
      "onFailure",
      workstation.onFailure,
      validPlaces,
      resourceNames,
      errors,
      locale,
    );
    validateWorkstationIORoutes(
      workstation,
      "onRejection",
      workstation.onRejection,
      validPlaces,
      resourceNames,
      errors,
      locale,
    );
  }
}

function validateWorkstationIORoutes(
  workstation: FactoryWorkstation,
  routeField: WorkstationIORouteField,
  routes: FactoryWorkstationIORoute,
  validPlaces: Set<string>,
  resourceNames: ReadonlySet<string>,
  errors: FactoryGraphDraftValidationError[],
  locale?: string | null,
) {
  for (const route of workstationIOEntries(routes)) {
    if (isInternalSystemTimeIO(route)) {
      continue;
    }
    if (isInternalResourceAvailabilityIO(route, resourceNames)) {
      continue;
    }

    const workTypeName = route.workType.trim();
    const stateName = route.state.trim();
    const placeKey = factoryWorkstationIOPlaceKey(workTypeName, stateName);
    if (validPlaces.has(placeKey)) {
      continue;
    }

    errors.push(
      unknownWorkstationRouteError({
        locale,
        routeField,
        stateName,
        workstationName: workstation.name,
        workTypeName,
      }),
    );
  }
}

function workstationIOEntries(
  routes: FactoryWorkstationIORoute,
): FactoryWorkstationIO[] {
  if (!routes) {
    return [];
  }
  return Array.isArray(routes) ? routes : [routes];
}

function collectFactoryGraphValidPlaces(
  factoryDefinition: CanonicalFactoryDefinition,
): Set<string> {
  const validPlaces = new Set<string>();

  for (const workType of factoryDefinition.workTypes ?? []) {
    if (isInternalSystemTimeWorkType(workType)) {
      continue;
    }

    for (const state of workType.states) {
      if (isInternalSystemTimeWorkState(state, workType.id ?? workType.name)) {
        continue;
      }

      validPlaces.add(factoryWorkstationIOPlaceKey(workType.name, state.name));
    }
  }

  return validPlaces;
}

function factoryWorkstationIOPlaceKey(
  workTypeName: string,
  stateName: string,
): string {
  return `${workTypeName}:${stateName}`;
}

function unknownWorkstationRouteError(input: {
  locale?: string | null;
  routeField: WorkstationIORouteField;
  stateName: string;
  workstationName: string;
  workTypeName: string;
}): FactoryGraphDraftValidationError {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const routeTargetId = nodeKeyId({
    kind: "work-state",
    stateName: input.stateName,
    workTypeName: input.workTypeName,
  });
  const workstationNodeId = nodeKeyId({
    kind: "workstation",
    name: input.workstationName,
  });

  return {
    code: "INVALID_WORKSTATION_ROUTE",
    message: messages.validationUnknownWorkstationRoute(input),
    target: {
      kind: "edge",
      id:
        input.routeField === "inputs"
          ? `${ROUTE_EDGE_KIND_BY_FIELD[input.routeField]}:${routeTargetId}->${workstationNodeId}`
          : `${ROUTE_EDGE_KIND_BY_FIELD[input.routeField]}:${workstationNodeId}->${routeTargetId}`,
    },
  };
}
