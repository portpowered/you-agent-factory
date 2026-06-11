import { useMemo } from "react";

import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useFactoryValidation } from "../../factory-graph-editor/hooks/validation/use-factory-validation";
import { buildDraftAppliedFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-apply";
import type { FactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { projectFactoryLayoutValidationTargets } from "../../factory-graph-editor/lib/layout/factory-graph-layout-validation";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";

export function useDraftAppliedFactoryValidation(
  draftState: EditableFactoryGraphViewModel["draftState"],
  layout: FactoryLayout | null,
  editorMode: boolean,
) {
  const draftAppliedFactoryDefinition = useMemo(() => {
    const baseDocument =
      draftState.latestDocument ?? draftState.baseDocument ?? null;
    if (!baseDocument) {
      return null;
    }

    return buildDraftAppliedFactoryDefinition(baseDocument, draftState.draft);
  }, [draftState.baseDocument, draftState.draft, draftState.latestDocument]);

  const apiValidation = useFactoryValidation(
    draftAppliedFactoryDefinition,
    editorMode,
  );
  const layoutValidationTargets = useMemo(() => {
    if (!editorMode || layout == null) {
      return [];
    }

    return projectFactoryLayoutValidationTargets(layout, draftState.graph);
  }, [draftState.graph, editorMode, layout]);
  const targets = useMemo(
    () => [...apiValidation.targets, ...layoutValidationTargets],
    [apiValidation.targets, layoutValidationTargets],
  );
  const projection = useMemo(
    () => projectFactoryValidationTargets(targets),
    [targets],
  );

  return {
    ...apiValidation,
    projection,
    targets,
  };
}
