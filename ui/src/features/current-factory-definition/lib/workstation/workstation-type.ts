import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  DEFAULT_WORKSTATION_TYPE,
  EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS,
  resolveEditableWorkstationTypeConversionOptions,
} from "../worker-workstation-taxonomy";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export type EditableWorkstationType = NonNullable<CanonicalWorkstation["type"]>;

export {
  DEFAULT_WORKSTATION_TYPE,
  EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS,
};

export function resolveEditableWorkstationType(
  workstation: Pick<CanonicalWorkstation, "type">,
): EditableWorkstationType {
  return workstation.type ?? DEFAULT_WORKSTATION_TYPE;
}

export function resolveEditableWorkstationTypeOptions(
  workstationType: EditableWorkstationType,
): readonly EditableWorkstationType[] {
  return resolveEditableWorkstationTypeConversionOptions(workstationType);
}

export function supportsEditableWorkstationTypeConversion(
  workstationType: EditableWorkstationType,
): boolean {
  return resolveEditableWorkstationTypeOptions(workstationType).length > 1;
}
