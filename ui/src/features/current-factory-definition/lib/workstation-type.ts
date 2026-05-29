import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export type EditableWorkstationType = NonNullable<CanonicalWorkstation["type"]>;

export const DEFAULT_WORKSTATION_TYPE: EditableWorkstationType =
  "MODEL_WORKSTATION";

export function resolveEditableWorkstationType(
  workstation: Pick<CanonicalWorkstation, "type">,
): EditableWorkstationType {
  return workstation.type ?? DEFAULT_WORKSTATION_TYPE;
}
