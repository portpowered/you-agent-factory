import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { preserveExistingBundledFilesWhenAbsent } from "../../../api/factory-definition";
import type { FactoryTimelineMode } from "../../timeline/state/factoryTimelineStore";
import { resolveObserveModeFactoryDefinition } from "./observe-mode-factory-definition";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";

function observeModeFactoryWithBundledDocs(
  factory: DashboardSnapshot["factory"] | null | undefined,
  document: ReturnType<
    typeof useCurrentActivityGraphEditor
  >["editableDefinitionQuery"]["data"],
) {
  if (!document) {
    return factory ?? undefined;
  }

  return preserveExistingBundledFilesWhenAbsent(factory ?? document, document);
}

function observeModeFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
  timelineMode: FactoryTimelineMode,
): DashboardSnapshot["factory"] | undefined {
  const document = editor.editableDefinitionQuery?.data;
  if (!document) {
    return snapshot.factory;
  }

  const resolvedFactory = resolveObserveModeFactoryDefinition({
    document,
    snapshotFactory: snapshot.factory,
    timelineMode,
  });

  return preserveExistingBundledFilesWhenAbsent(resolvedFactory, document);
}

function editorModeFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  return (
    editor.draftState.pendingFactoryDefinition ??
    editor.draftState.latestDocument ??
    editor.draftState.baseDocument ??
    undefined
  );
}

export function currentActivityCardFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
  timelineMode: FactoryTimelineMode,
): DashboardSnapshot["factory"] | null | undefined {
  if (!editor.editorMode) {
    const document = editor.editableDefinitionQuery?.data;
    if (editor.editableDefinitionQuery?.status !== "success") {
      return observeModeFactoryWithBundledDocs(snapshot.factory, document) ?? null;
    }

    return observeModeFactoryDefinition(editor, snapshot, timelineMode);
  }

  if (editor.editableDefinitionQuery?.status !== "success") {
    return null;
  }

  return editorModeFactoryDefinition(editor) ?? null;
}

export function currentActivityCardDisplayFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
  timelineMode: FactoryTimelineMode,
): DashboardSnapshot["factory"] | null | undefined {
  const document = editor.editableDefinitionQuery?.data;
  const resolvedFactory = currentActivityCardFactoryDefinition(
    editor,
    snapshot,
    timelineMode,
  );

  if (editor.editorMode) {
    return resolvedFactory ?? undefined;
  }

  return observeModeFactoryWithBundledDocs(resolvedFactory, document);
}
