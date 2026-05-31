import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";

type CanonicalWorkType = NonNullable<
  CanonicalFactoryDefinition["workTypes"]
>[number];
type CanonicalWorkState = CanonicalWorkType["states"][number];
type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorkstationIO = CanonicalWorkstation["inputs"][number];
type WorkStateType = CanonicalWorkState["type"];

export interface EditableWorkStateValues {
  stateName: string;
  stateNamesInWorkType: string[];
  stateType: WorkStateType;
  workTypeName: string;
}

export interface EditableWorkStateDraft {
  name: string;
  type: WorkStateType;
}

export function resolveEditableWorkStateValues(
  factory: CanonicalFactoryDefinition,
  placeId: string,
): EditableWorkStateValues | null {
  const resolution = resolveWorkStateFromPlaceId(factory, placeId);
  if (!resolution) {
    return null;
  }

  const { workType, state } = resolution;

  return {
    stateName: state.name,
    stateNamesInWorkType: workType.states.map((entry) => entry.name),
    stateType: state.type,
    workTypeName: workType.name,
  };
}

export function editableWorkStateDraftFromValues(
  values: EditableWorkStateValues,
): EditableWorkStateDraft {
  return {
    name: values.stateName,
    type: values.stateType,
  };
}

export function applyEditableWorkStateDraft(
  factory: CanonicalFactoryDefinition,
  placeId: string,
  draft: EditableWorkStateDraft,
): CanonicalFactoryDefinition | null {
  const resolution = resolveWorkStateFromPlaceId(factory, placeId);
  if (!resolution) {
    return null;
  }

  const trimmedName = draft.name.trim();
  const originalStateName = resolution.state.name;
  const { workType, workTypeIndex, stateIndex } = resolution;

  const nextWorkTypes = (factory.workTypes ?? []).map(
    (workTypeEntry, index) => {
      if (index !== workTypeIndex) {
        return workTypeEntry;
      }

      return {
        ...workTypeEntry,
        states: workTypeEntry.states.map((state, stateEntryIndex) =>
          stateEntryIndex === stateIndex
            ? {
                ...state,
                name: trimmedName,
                type: draft.type,
              }
            : state,
        ),
      };
    },
  );

  const nextWorkstations = (factory.workstations ?? []).map((workstation) =>
    rewriteWorkstationStateReferences(
      workstation,
      workType.name,
      originalStateName,
      trimmedName,
    ),
  );

  return {
    ...factory,
    workTypes: nextWorkTypes,
    workstations: nextWorkstations,
  };
}

function resolveWorkStateFromPlaceId(
  factory: CanonicalFactoryDefinition,
  placeId: string,
): {
  state: CanonicalWorkState;
  stateIndex: number;
  workType: CanonicalWorkType;
  workTypeIndex: number;
} | null {
  const workTypes = factory.workTypes ?? [];

  for (const [workTypeIndex, workType] of workTypes.entries()) {
    for (const [stateIndex, state] of workType.states.entries()) {
      if (workStatePlaceId(workType.name, state.name) !== placeId) {
        continue;
      }

      return {
        state,
        stateIndex,
        workType,
        workTypeIndex,
      };
    }
  }

  return null;
}

function workStatePlaceId(workTypeName: string, stateName: string): string {
  return `${workTypeName}:${stateName}`;
}

function rewriteWorkstationStateReferences(
  workstation: CanonicalWorkstation,
  workTypeName: string,
  originalStateName: string,
  nextStateName: string,
): CanonicalWorkstation {
  return {
    ...workstation,
    classificationRoutes: workstation.classificationRoutes?.map((route) => ({
      ...route,
      outputs: rewriteRequiredWorkstationIORoutes(
        route.outputs,
        workTypeName,
        originalStateName,
        nextStateName,
      ),
    })),
    inputs: rewriteRequiredWorkstationIORoutes(
      workstation.inputs,
      workTypeName,
      originalStateName,
      nextStateName,
    ),
    onContinue: rewriteOptionalWorkstationIORoutes(
      workstation.onContinue,
      workTypeName,
      originalStateName,
      nextStateName,
    ),
    onFailure: rewriteOptionalWorkstationIORoutes(
      workstation.onFailure,
      workTypeName,
      originalStateName,
      nextStateName,
    ),
    onRejection: rewriteOptionalWorkstationIORoutes(
      workstation.onRejection,
      workTypeName,
      originalStateName,
      nextStateName,
    ),
    outputs: rewriteOptionalWorkstationIORoutes(
      workstation.outputs,
      workTypeName,
      originalStateName,
      nextStateName,
    ),
  };
}

function rewriteRequiredWorkstationIORoutes(
  routes: CanonicalWorkstationIO[],
  workTypeName: string,
  originalStateName: string,
  nextStateName: string,
): CanonicalWorkstationIO[] {
  return routes.map((route) =>
    route.workType === workTypeName && route.state === originalStateName
      ? { ...route, state: nextStateName }
      : route,
  );
}

function rewriteOptionalWorkstationIORoutes(
  routes: CanonicalWorkstationIO[] | undefined,
  workTypeName: string,
  originalStateName: string,
  nextStateName: string,
): CanonicalWorkstationIO[] | undefined {
  if (!routes) {
    return routes;
  }

  return rewriteRequiredWorkstationIORoutes(
    routes,
    workTypeName,
    originalStateName,
    nextStateName,
  );
}
