import { useMemo } from "react";

import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useFactoryValidation } from "../../factory-graph-editor/hooks/use-factory-validation";
import { buildDraftAppliedFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-apply";

export function useDraftAppliedFactoryValidation(
  draftState: EditableFactoryGraphViewModel["draftState"],
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

  return useFactoryValidation(draftAppliedFactoryDefinition, editorMode);
}
