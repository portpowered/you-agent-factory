import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  workerNamesReferencingResourceInFactoryDefinition,
  workstationNamesReferencingResourceInFactoryDefinition,
} from "../../current-selection/resource-selection/lib/resource-detail-values";

type CanonicalResource = NonNullable<CanonicalFactoryDefinition["resources"]>[number];
type ResourceType = NonNullable<CanonicalResource["type"]>;

export const EDITABLE_RESOURCE_TYPES: ResourceType[] = [
  "MODEL",
  "PROVIDER_QUOTA",
  "INVOCATION_SLOT",
];

export interface EditableResourceValues {
  backend: string | null;
  capacity: number;
  loadPolicy: string | null;
  model: string | null;
  provider: string | null;
  resourceName: string;
  type: ResourceType | undefined;
  workerNames: string[];
  workstationNames: string[];
}

export interface EditableResourceDraft {
  backend: string;
  capacityText: string;
  loadPolicy: string;
  model: string;
  name: string;
  provider: string;
  type: ResourceType | null;
}

export function resolveEditableResourceValues(
  factory: CanonicalFactoryDefinition,
  resourceName: string,
): EditableResourceValues | null {
  const resourceResolution = resolveCanonicalResource(factory, resourceName);
  if (!resourceResolution) {
    return null;
  }

  const { resource } = resourceResolution;

  return {
    backend: resource.backend ?? null,
    capacity: resource.capacity,
    loadPolicy: resource.loadPolicy ?? null,
    model: resource.model ?? null,
    provider: resource.provider ?? null,
    resourceName: resource.name,
    type: resource.type,
    workerNames: workerNamesReferencingResourceInFactoryDefinition(
      factory,
      resourceName,
    ),
    workstationNames: workstationNamesReferencingResourceInFactoryDefinition(
      factory,
      resourceName,
    ),
  };
}

export function editableResourceDraftFromValues(
  values: EditableResourceValues,
): EditableResourceDraft {
  return {
    backend: values.backend ?? "",
    capacityText: String(values.capacity),
    loadPolicy: values.loadPolicy ?? "",
    model: values.model ?? "",
    name: values.resourceName,
    provider: values.provider ?? "",
    type: values.type ?? null,
  };
}

export function applyEditableResourceDraft(
  factory: CanonicalFactoryDefinition,
  resourceName: string,
  draft: EditableResourceDraft,
): CanonicalFactoryDefinition | null {
  const resourceResolution = resolveCanonicalResource(factory, resourceName);
  if (!resourceResolution || !factory.resources) {
    return null;
  }

  const trimmedName = draft.name.trim();
  const nextResource = buildResourceFromDraft(resourceResolution.resource, draft);

  return {
    ...factory,
    resources: factory.resources.map((resource, index) =>
      index === resourceResolution.resourceIndex ? nextResource : resource,
    ),
    workers: (factory.workers ?? []).map((worker) => ({
      ...worker,
      resources: (worker.resources ?? []).map((requirement) =>
        requirement.name === resourceName
          ? { ...requirement, name: trimmedName }
          : requirement,
      ),
    })),
    workstations: (factory.workstations ?? []).map((workstation) => ({
      ...workstation,
      resources: (workstation.resources ?? []).map((requirement) =>
        requirement.name === resourceName
          ? { ...requirement, name: trimmedName }
          : requirement,
      ),
    })),
  };
}

export function parseResourceCapacityText(
  capacityText: string,
): number | null {
  const trimmed = capacityText.trim();
  if (trimmed.length === 0) {
    return null;
  }

  const capacity = Number.parseInt(trimmed, 10);
  if (!Number.isInteger(capacity)) {
    return null;
  }

  return capacity;
}

function buildResourceFromDraft(
  existingResource: CanonicalResource,
  draft: EditableResourceDraft,
): CanonicalResource {
  const capacity = parseResourceCapacityText(draft.capacityText);
  const base: CanonicalResource = {
    name: draft.name.trim(),
    capacity: capacity ?? existingResource.capacity,
  };

  if (!draft.type) {
    return base;
  }

  const typedBase: CanonicalResource = {
    ...base,
    type: draft.type,
  };

  if (draft.type === "MODEL") {
    return {
      ...typedBase,
      ...(draft.model.trim().length > 0 ? { model: draft.model.trim() } : {}),
      ...(draft.backend.trim().length > 0
        ? { backend: draft.backend.trim() }
        : {}),
      ...(draft.loadPolicy.trim().length > 0
        ? { loadPolicy: draft.loadPolicy.trim() }
        : {}),
    };
  }

  if (draft.type === "PROVIDER_QUOTA") {
    return {
      ...typedBase,
      ...(draft.provider.trim().length > 0
        ? { provider: draft.provider.trim() }
        : {}),
    };
  }

  return typedBase;
}

function resolveCanonicalResource(
  factory: CanonicalFactoryDefinition,
  resourceName: string,
): { resource: CanonicalResource; resourceIndex: number } | null {
  const resources = factory.resources ?? [];
  const resourceIndex = resources.findIndex(
    (resource) => resource.name === resourceName,
  );
  if (resourceIndex < 0) {
    return null;
  }

  return {
    resource: resources[resourceIndex],
    resourceIndex,
  };
}
