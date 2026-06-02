import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import type { DashboardSnapshot } from "../../../../api/dashboard/types";
import type { FactoryResource } from "../../../../api/events/types";

function referencesResourceName(
  requirements: ReadonlyArray<{ name: string }> | undefined,
  resourceName: string,
): boolean {
  return (requirements ?? []).some(
    (requirement) => requirement.name === resourceName,
  );
}

export function resourceAvailablePlaceId(resourceName: string): string {
  return `${resourceName}:available`;
}

export function resourceTokenCountFromSnapshot(
  snapshot: DashboardSnapshot | null | undefined,
  resourceName: string,
): number | null {
  if (!snapshot) {
    return null;
  }

  const placeId = resourceAvailablePlaceId(resourceName);
  const counts = snapshot.runtime.place_token_counts;
  if (!counts || !(placeId in counts)) {
    return null;
  }

  return counts[placeId] ?? 0;
}

export function findResourceInFactoryDefinition(
  factory: CanonicalFactoryDefinition,
  resourceName: string,
): FactoryResource | undefined {
  return (factory.resources ?? []).find(
    (resource) => resource.name === resourceName,
  );
}

export function workerNamesReferencingResourceInFactoryDefinition(
  factory: CanonicalFactoryDefinition,
  resourceName: string,
): string[] {
  return (factory.workers ?? [])
    .filter((worker) => referencesResourceName(worker.resources, resourceName))
    .map((worker) => worker.name)
    .filter((name) => name.length > 0);
}

export function workstationNamesReferencingResourceInFactoryDefinition(
  factory: CanonicalFactoryDefinition,
  resourceName: string,
): string[] {
  return (factory.workstations ?? [])
    .filter((workstation) =>
      referencesResourceName(workstation.resources, resourceName),
    )
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}

export function resourceShowsModelFields(resource: FactoryResource): boolean {
  return resource.type === "MODEL";
}

export function resourceShowsProviderQuotaFields(
  resource: FactoryResource,
): boolean {
  return resource.type === "PROVIDER_QUOTA";
}
