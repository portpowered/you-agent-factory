import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";

type CanonicalWorkType = NonNullable<
  CanonicalFactoryDefinition["workTypes"]
>[number];
type WorkTypeHandlingBehavior = NonNullable<
  CanonicalWorkType["handlingBehavior"]
>[number];
type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type WorkstationIO = CanonicalWorkstation["inputs"][number];

export interface EditableWorkTypeValues {
  handlingBehavior: WorkTypeHandlingBehavior[] | undefined;
  states: CanonicalWorkType["states"];
  workTypeName: string;
}

export interface EditableWorkTypeDraft {
  handlingBehavior: WorkTypeHandlingBehavior[] | null;
  name: string;
}

export function resolveEditableWorkTypeValues(
  factory: CanonicalFactoryDefinition,
  workTypeName: string,
): EditableWorkTypeValues | null {
  const workTypeResolution = resolveCanonicalWorkType(factory, workTypeName);
  if (!workTypeResolution) {
    return null;
  }

  const { workType } = workTypeResolution;

  return {
    handlingBehavior: workType.handlingBehavior,
    states: workType.states,
    workTypeName: workType.name,
  };
}

export function editableWorkTypeDraftFromValues(
  values: EditableWorkTypeValues,
): EditableWorkTypeDraft {
  return {
    handlingBehavior: values.handlingBehavior ?? null,
    name: values.workTypeName,
  };
}

export function applyEditableWorkTypeDraft(
  factory: CanonicalFactoryDefinition,
  workTypeName: string,
  draft: EditableWorkTypeDraft,
): CanonicalFactoryDefinition | null {
  const workTypeResolution = resolveCanonicalWorkType(factory, workTypeName);
  if (!workTypeResolution || !factory.workTypes) {
    return null;
  }

  const trimmedName = draft.name.trim();
  const nextWorkType = buildWorkTypeFromDraft(
    workTypeResolution.workType,
    draft,
  );
  const renamed = trimmedName !== workTypeName;

  return {
    ...factory,
    workTypes: factory.workTypes.map((workType, index) =>
      index === workTypeResolution.workTypeIndex ? nextWorkType : workType,
    ),
    workstations: renamed
      ? (factory.workstations ?? []).map((workstation) =>
          rewriteWorkstationWorkTypeReferences(
            workstation,
            workTypeName,
            trimmedName,
          ),
        )
      : factory.workstations,
  };
}

function buildWorkTypeFromDraft(
  existingWorkType: CanonicalWorkType,
  draft: EditableWorkTypeDraft,
): CanonicalWorkType {
  const trimmedName = draft.name.trim();
  const nextWorkType: CanonicalWorkType = {
    name: trimmedName,
    states: existingWorkType.states,
  };

  if (draft.handlingBehavior && draft.handlingBehavior.length > 0) {
    nextWorkType.handlingBehavior = draft.handlingBehavior;
  }

  return nextWorkType;
}

function rewriteWorkstationWorkTypeReferences(
  workstation: CanonicalWorkstation,
  previousWorkTypeName: string,
  nextWorkTypeName: string,
): CanonicalWorkstation {
  return {
    ...workstation,
    inputs: rewriteWorkstationIORoutes(
      workstation.inputs,
      previousWorkTypeName,
      nextWorkTypeName,
    ),
    ...(workstation.outputs
      ? {
          outputs: rewriteWorkstationIORoutes(
            workstation.outputs,
            previousWorkTypeName,
            nextWorkTypeName,
          ),
        }
      : {}),
    ...(workstation.onContinue
      ? {
          onContinue: rewriteWorkstationIORoutes(
            workstation.onContinue,
            previousWorkTypeName,
            nextWorkTypeName,
          ),
        }
      : {}),
    ...(workstation.onRejection
      ? {
          onRejection: rewriteWorkstationIORoutes(
            workstation.onRejection,
            previousWorkTypeName,
            nextWorkTypeName,
          ),
        }
      : {}),
    ...(workstation.onFailure
      ? {
          onFailure: rewriteWorkstationIORoutes(
            workstation.onFailure,
            previousWorkTypeName,
            nextWorkTypeName,
          ),
        }
      : {}),
    ...(workstation.classificationRoutes
      ? {
          classificationRoutes: workstation.classificationRoutes.map(
            (route) => ({
              ...route,
              outputs: rewriteWorkstationIORoutes(
                route.outputs,
                previousWorkTypeName,
                nextWorkTypeName,
              ),
            }),
          ),
        }
      : {}),
  };
}

function rewriteWorkstationIORoutes(
  routes: WorkstationIO[],
  previousWorkTypeName: string,
  nextWorkTypeName: string,
): WorkstationIO[] {
  return routes.map((route) =>
    route.workType === previousWorkTypeName
      ? { ...route, workType: nextWorkTypeName }
      : route,
  );
}

function resolveCanonicalWorkType(
  factory: CanonicalFactoryDefinition,
  workTypeName: string,
): { workType: CanonicalWorkType; workTypeIndex: number } | null {
  const workTypes = factory.workTypes ?? [];
  const workTypeIndex = workTypes.findIndex(
    (workType) => workType.name === workTypeName,
  );
  if (workTypeIndex < 0) {
    return null;
  }

  return {
    workType: workTypes[workTypeIndex],
    workTypeIndex,
  };
}
