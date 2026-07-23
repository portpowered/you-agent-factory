import { useMemo } from "react";
import {
  type FactoryBundledDocFile,
  factoryBundledDocDisplayLabel,
  findFactoryBundledDocFile,
} from "../../../workflow-activity/lib/factory-bundled-docs";
import { useGraphEditorPendingFactoryBridge } from "../../../workflow-activity/state/graph-editor-pending-factory-bridge";

export type DocDetailState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty" }
  | {
      status: "ready";
      displayLabel: string;
      inlineContent: string;
      targetPath: string;
    };

export function useDocDetailState(
  {
    savedBundledDoc,
    targetPath,
  }: {
    savedBundledDoc?: FactoryBundledDocFile | null;
    targetPath: string;
  },
  _locale?: string | null,
): DocDetailState {
  const pendingFactoryDefinition = useGraphEditorPendingFactoryBridge(
    (state) => state.pendingFactoryDefinition,
  );

  return useMemo((): DocDetailState => {
    const bundledFile =
      savedBundledDoc ??
      findFactoryBundledDocFile(pendingFactoryDefinition, targetPath);

    if (!bundledFile) {
      return { status: "empty" };
    }

    return {
      status: "ready",
      displayLabel: factoryBundledDocDisplayLabel(targetPath),
      inlineContent: bundledFile.content?.inline ?? "",
      targetPath,
    };
  }, [pendingFactoryDefinition, savedBundledDoc, targetPath]);
}
