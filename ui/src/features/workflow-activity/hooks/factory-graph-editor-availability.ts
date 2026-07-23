import { WorkstationType } from "../../../api/generated/openapi";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";

export function findClassifierGraphEditorUnsupportedWorkstationName(
  factoryDefinition: CanonicalFactoryDefinition | null,
): string | undefined {
  const classifierWorkstation = factoryDefinition?.workstations?.find(
    (workstation) =>
      workstation.type === WorkstationType.CLASSIFIER_WORKSTATION ||
      (workstation.classificationRoutes?.length ?? 0) > 0,
  );

  if (!classifierWorkstation) {
    return undefined;
  }

  return classifierWorkstation.name;
}
