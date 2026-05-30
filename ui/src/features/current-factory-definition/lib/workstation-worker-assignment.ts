import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { resolveEditableWorkstationType } from "./workstation-type";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export function workstationRequiresWorkerAssignment(
  workstation: Pick<CanonicalWorkstation, "type">,
): boolean {
  return resolveEditableWorkstationType(workstation) !== "LOGICAL_MOVE";
}
