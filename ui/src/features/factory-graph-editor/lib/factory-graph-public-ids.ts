import type {
  CanonicalFactoryDefinition,
  FactoryResource,
  FactoryWorker,
  FactoryWorkState,
  FactoryWorkstation,
  FactoryWorkType,
} from "./factory-graph-draft-types";

type GraphableNamedEntity =
  | FactoryResource
  | FactoryWorker
  | FactoryWorkState
  | FactoryWorkstation
  | FactoryWorkType;

export function materializeFactoryGraphEntityIdsForSave(
  factoryDefinition: CanonicalFactoryDefinition,
): CanonicalFactoryDefinition {
  const nextFactoryDefinition = structuredClone(factoryDefinition);

  materializeNamedEntityIds(nextFactoryDefinition.resources);
  materializeNamedEntityIds(nextFactoryDefinition.workers);
  materializeNamedEntityIds(nextFactoryDefinition.workstations);

  for (const workType of nextFactoryDefinition.workTypes ?? []) {
    materializeNamedEntityId(workType);
    materializeNamedEntityIds(workType.states);
  }

  return nextFactoryDefinition;
}

function materializeNamedEntityIds(
  entities: GraphableNamedEntity[] | undefined,
): void {
  for (const entity of entities ?? []) {
    materializeNamedEntityId(entity);
  }
}

function materializeNamedEntityId(entity: GraphableNamedEntity): void {
  if (typeof entity.id === "string" && entity.id.trim().length > 0) {
    return;
  }

  entity.id = entity.name;
}
