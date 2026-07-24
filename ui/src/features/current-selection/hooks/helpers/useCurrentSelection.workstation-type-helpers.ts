import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import type {
  DashboardSnapshot,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";

export function resolveWorkstationTypeForRequest(
  workstationName: string | undefined,
  factory: CurrentFactoryDocument | null | undefined,
): string | undefined {
  const normalizedName = workstationName?.trim();
  if (!normalizedName || !factory?.workstations?.length) {
    return undefined;
  }

  return factory.workstations.find(
    (workstation) => workstation.name === normalizedName,
  )?.type;
}

export function enrichWorkstationRequestWithWorkstationType(
  request: DashboardWorkstationRequest,
  snapshot: DashboardSnapshot | null | undefined,
): DashboardWorkstationRequest {
  if (request.workstation_type) {
    return request;
  }

  const workstationType = resolveWorkstationTypeForRequest(
    request.workstation_name,
    snapshot?.factory as CurrentFactoryDocument | null | undefined,
  );
  if (!workstationType) {
    return request;
  }

  return { ...request, workstation_type: workstationType };
}
