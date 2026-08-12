import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { WorkstationType } from "../../../api/generated/openapi";
import { isHumanApprovalWorkstationType } from "./worker-workstation-taxonomy";
import { resolveEditableWorkstationType } from "./workstation/workstation-type";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];

export function workstationRequiresWorkerAssignment(
  workstation: Pick<CanonicalWorkstation, "type">,
): boolean {
  return (
    resolveEditableWorkstationType(workstation) !==
      WorkstationType.LOGICAL_MOVE &&
    !isHumanApprovalWorkstationType(resolveEditableWorkstationType(workstation))
  );
}
