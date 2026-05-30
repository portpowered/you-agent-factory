import { useFactoryImportActivationTarget } from "../../import/hooks/use-factory-import-activation-target";
import { useFactoryImportSaveChoice } from "../../import/hooks/use-factory-import-save-choice";
import type { useCurrentActivityImportController } from "../../workflow-activity/hooks/current-activity-import-controller";

export function useDashboardBentoImport({
  importController,
  sessionID,
}: {
  importController: ReturnType<typeof useCurrentActivityImportController>;
  sessionID: string | null | undefined;
}) {
  const readyImportPreview =
    importController.importPreviewState.status === "ready"
      ? importController.importPreviewState
      : null;
  const [importSaveChoice, setImportSaveChoice] =
    useFactoryImportSaveChoice(readyImportPreview);
  const importActivationTarget = useFactoryImportActivationTarget({
    enabled: readyImportPreview !== null,
    preferredFactoryName: readyImportPreview?.value?.factory?.name,
    sessionID,
  });

  return { importActivationTarget, importSaveChoice, setImportSaveChoice };
}
