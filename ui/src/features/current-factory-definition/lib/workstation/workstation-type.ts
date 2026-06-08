import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import { WorkstationType } from "../../../../api/generated/openapi";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export type EditableWorkstationType = NonNullable<CanonicalWorkstation["type"]>;

export const DEFAULT_WORKSTATION_TYPE: EditableWorkstationType =
  WorkstationType.WorkstationTypeModelWorkstation;

export function resolveEditableWorkstationType(
  workstation: Pick<CanonicalWorkstation, "type">,
): EditableWorkstationType {
  return workstation.type ?? DEFAULT_WORKSTATION_TYPE;
}

export const EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS = [
  WorkstationType.WorkstationTypeModelWorkstation,
  WorkstationType.WorkstationTypeModelInvoke,
] as const satisfies readonly EditableWorkstationType[];

export function resolveEditableWorkstationTypeOptions(
  workstationType: EditableWorkstationType,
): readonly EditableWorkstationType[] {
  if (workstationType === WorkstationType.WorkstationTypeLogicalMove) {
    return [WorkstationType.WorkstationTypeLogicalMove];
  }

  return EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS;
}

export function supportsEditableWorkstationTypeConversion(
  workstationType: EditableWorkstationType,
): boolean {
  return resolveEditableWorkstationTypeOptions(workstationType).length > 1;
}
