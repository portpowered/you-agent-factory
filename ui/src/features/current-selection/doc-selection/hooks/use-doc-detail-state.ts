import { useMemo } from "react";

import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { factoryBundledDocDisplayLabel } from "../../../workflow-activity/lib/factory-bundled-docs";
import { getDocDetailMessages } from "../messages/doc-detail";

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
  targetPath: string,
  locale?: string | null,
): DocDetailState {
  const documentQuery = useCurrentFactoryDocument();
  const messages = getDocDetailMessages(locale);

  return useMemo((): DocDetailState => {
    if (documentQuery.status === "pending") {
      return { status: "loading" };
    }

    if (documentQuery.status === "error") {
      return {
        status: "error",
        errorMessage:
          documentQuery.error?.message ?? messages.configurationUnknownError,
      };
    }

    const bundledFile = documentQuery.data?.supportingFiles?.bundledFiles?.find(
      (candidate) =>
        candidate.type === "DOC" && candidate.targetPath === targetPath,
    );

    if (!bundledFile) {
      return { status: "empty" };
    }

    return {
      status: "ready",
      displayLabel: factoryBundledDocDisplayLabel(targetPath),
      inlineContent: bundledFile.content?.inline ?? "",
      targetPath,
    };
  }, [
    documentQuery.data,
    documentQuery.error,
    documentQuery.status,
    messages.configurationUnknownError,
    targetPath,
  ]);
}
