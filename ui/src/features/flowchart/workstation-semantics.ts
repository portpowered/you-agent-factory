import type { DashboardWorkstationNode } from "../../api/dashboard/types";

export const EXHAUSTION_WORKSTATION_KIND = "exhaustion";

export function isExhaustionWorkstation(workstation: DashboardWorkstationNode): boolean {
  return (
    workstation.workstation_kind === EXHAUSTION_WORKSTATION_KIND ||
    ((workstation.workstation_kind ?? "") === "" && (workstation.worker_type ?? "") === "")
  );
}

