import type {
  CanonicalFactoryDefinition,
  FactoryGraphWorkStateReference,
  FactoryWorkState,
} from "../draft/factory-graph-draft-types";

export type FactoryGraphWorkStateType =
  | FactoryWorkState["type"]
  | (string & {});

/**
 * Resolves the canonical lifecycle type for a work-state graph node from the
 * factory definition. Returns undefined when the work type or state is missing.
 */
export function resolveWorkStateTypeForGraphNode(
  factoryDefinition: CanonicalFactoryDefinition | null | undefined,
  workStateKey: FactoryGraphWorkStateReference,
): FactoryGraphWorkStateType | undefined {
  if (!factoryDefinition) {
    return undefined;
  }

  const workType = factoryDefinition.workTypes?.find(
    (entry) => entry.name === workStateKey.workTypeName,
  );
  const workState = workType?.states.find(
    (entry) => entry.name === workStateKey.stateName,
  );

  return workState?.type;
}
